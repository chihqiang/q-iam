package logic

import (
	"context"
	"sync"
	"time"

	"chihqiang/q-iam/model"

	"github.com/chihqiang/infra-go/logger"
	"gorm.io/gorm"
)

// 审计落库队列参数：批量落库，避免每个写请求一个 goroutine + 一次 INSERT。
const (
	// auditQueueSize 审计队列缓冲大小，超过后进入背压等待。
	auditQueueSize = 1024
	// auditBatchSize 每批落库条数，达到即刷盘。
	auditBatchSize = 50
	// auditFlushInterval 兑底定时刷盘间隔（防止低流量下日志滞留队列）。
	auditFlushInterval = 200 * time.Millisecond
	// auditEnqueueTimeout 队列满时阻塞等待入队的最大时长；超时仍未入队才丢弃并告警。
	// 阻塞发生在响应已写出之后（审计在 next 之后执行），对业务吞吐影响可控，
	// 相比原先直接丢弃大幅降低审计丢失概率。
	auditEnqueueTimeout = 5 * time.Second
)

// AuditLogic 操作审计逻辑：记录与查询操作审计日志。
// 落库采用「有缓冲队列 + 单 worker 批量写入」：
//   - 请求侧 Record 非阻塞入队，不拖慢业务；
//   - worker 攒批/定时批量插入，避免并发写库连接不受控。
type AuditLogic struct {
	db    *gorm.DB
	queue chan *model.AuditLog
	wg    sync.WaitGroup
	once  sync.Once
}

// NewAuditLogic 创建操作审计逻辑（启动落库 worker）。
func NewAuditLogic(db *gorm.DB) *AuditLogic {
	s := &AuditLogic{
		db:    db,
		queue: make(chan *model.AuditLog, auditQueueSize),
	}
	s.wg.Add(1)
	go s.flushLoop()
	return s
}

// Close 停止落库 worker 并排空队列（服务优雅关闭时调用）。
func (s *AuditLogic) Close() {
	s.once.Do(func() {
		close(s.queue)
		s.wg.Wait()
	})
}

// AuditEntry 一条审计记录（记录方法参数）。
type AuditEntry struct {
	OperatorID   int64
	OperatorName string
	Module       string
	Action       string
	Method       string
	Path         string
	Detail       string
	ClientIP     string
	UserAgent    string
	Success      bool
	ErrorMsg     string
	LatencyMs    int64
}

// Record 记录一条操作审计日志（异步落库，不阻塞请求，失败仅记日志）。
func (s *AuditLogic) Record(ctx context.Context, entry AuditEntry) {
	if entry.Module == "" {
		entry.Module = "system"
	}
	if entry.Action == "" {
		entry.Action = "unknown"
	}
	log := model.AuditLog{
		OperatorID:   entry.OperatorID,
		OperatorName: entry.OperatorName,
		Module:       entry.Module,
		Action:       entry.Action,
		Method:       entry.Method,
		Path:         entry.Path,
		Detail:       entry.Detail,
		ClientIP:     entry.ClientIP,
		UserAgent:    entry.UserAgent,
		Success:      entry.Success,
		ErrorMsg:     entry.ErrorMsg,
		LatencyMs:    entry.LatencyMs,
	}
	// 背压入队：队列满时阻塞等待（最长达 auditEnqueueTimeout），超时才丢弃并告警。
	// 相比原先立即丢弃，大幅降低审计丢失概率；阻塞发生在响应已写出之后，对业务影响可控。
	if !s.enqueue(ctx, &log) {
		logger.WarnCtx(ctx, "audit queue full, audit log dropped after timeout",
			logger.String("module", entry.Module),
			logger.String("action", entry.Action))
	}
}

// enqueue 将审计日志入队。返回是否成功入队。
// 队列满时最多等待 auditEnqueueTimeout；服务关闭（队列已 close）时返回 false。
func (s *AuditLogic) enqueue(ctx context.Context, log *model.AuditLog) (ok bool) {
	defer func() {
		// 优雅关闭时队列可能已 close，发送会 panic，捕获后视为入队失败
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case s.queue <- log:
		return true
	case <-time.After(auditEnqueueTimeout):
		return false
	}
}

// flushLoop 消费队列并批量落库。
func (s *AuditLogic) flushLoop() {
	defer s.wg.Done()

	batch := make([]*model.AuditLog, 0, auditBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		logs := batch
		batch = make([]*model.AuditLog, 0, auditBatchSize)
		cctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.db.WithContext(cctx).Create(&logs).Error; err != nil {
			logger.ErrorCtx(context.Background(), "audit flush failed",
				logger.Err(err), logger.Int("count", len(logs)))
		}
	}

	ticker := time.NewTicker(auditFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case log, ok := <-s.queue:
			if !ok {
				// 队列关闭：排空剩余后退出
				flush()
				return
			}
			batch = append(batch, log)
			if len(batch) >= auditBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// AuditListRequest 审计日志列表请求。
type AuditListRequest struct {
	PageRequest
	Key      string `form:"key"`
	Module   string `form:"module"`
	Action   string `form:"action"`
	Success  *bool  `form:"success"`
	Operator string `form:"operator"`
	From     string `form:"from"` // RFC3339
	To       string `form:"to"`   // RFC3339
}

// List 分页查询审计日志。
func (s *AuditLogic) List(ctx context.Context, req *AuditListRequest) (*PageResponse[model.AuditLog], error) {
	var logs []model.AuditLog
	var total int64

	query := s.db.WithContext(ctx).Model(&model.AuditLog{})
	if req.Module != "" {
		query = query.Where("module = ?", req.Module)
	}
	if req.Action != "" {
		query = query.Where("action = ?", req.Action)
	}
	if req.Success != nil {
		query = query.Where("success = ?", *req.Success)
	}
	if req.Operator != "" {
		query = query.Where("operator_name = ?", req.Operator)
	}
	if req.Key != "" {
		like := "%" + req.Key + "%"
		query = query.Where("detail LIKE ? OR path LIKE ? OR operator_name LIKE ?", like, like, like)
	}
	if req.From != "" {
		if t, err := time.Parse(time.RFC3339, req.From); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if req.To != "" {
		if t, err := time.Parse(time.RFC3339, req.To); err == nil {
			query = query.Where("created_at <= ?", t)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (req.Page - 1) * req.Size
	if err := query.Order("id DESC").Offset(offset).Limit(req.Size).Find(&logs).Error; err != nil {
		return nil, err
	}

	return &PageResponse[model.AuditLog]{Data: logs, Total: total}, nil
}

// AuditModuleOptions 审计模块枚举（供前端筛选）。
// 含 system（历史数据清理等系统操作），与路由声明式审计模块保持一致。
var AuditModuleOptions = []string{"auth", "account", "group", "policy", "grant", "app", "oauth", "system"}
