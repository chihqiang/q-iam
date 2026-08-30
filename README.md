# q-iam

**身份与访问管理（IAM）系统**——管理账号、账号组、权限策略、授权关系，并提供 **OAuth2 授权服务器能力**（第三方应用作为客户端接入）。

---

## ✨ 核心功能

### 身份管理（Identity）

- **账号**：CRUD、启用/禁用、进入控制台开关、所属账号组、登录失败锁定
- **账号组**：CRUD、组内成员批量增删替换

### 权限管理（Permission）

- **策略**：系统内置策略（`AdministratorAccess`，不可改删）+ 自定义策略
- **授权语句（语句池）**：语句独立菜单管理，可被多个策略共享引用；策略新增/编辑只负责关联已有语句（`statement_ids`），语句变更后关联策略即时生效
- **授权规则**：语句（Statement）→ 数据范围（DataScope），支持 `Allow`/`Deny`、`*` 通配
- **授权**：策略绑定到 账号/账号组/应用 三种主体
- **数据权限**：`all` / `group` / `self` / `attribute` 四种数据范围
- **判定语义**：显式 `Deny` 优先；权限集始终缓存（无 Redis 用进程内 MemCache，配置 Redis 后切换 RedisCache），授权/语句变更即时失效

### 认证与安全（AuthN/AuthZ）

- 登录 / 注册（可开关）/ 令牌刷新（轮换 + 重用检测）/ 应用换取令牌
- JWT（HS256），密钥支持环境变量覆盖
- 密码策略（长度、大小写、数字、特殊字符、禁含账号名等）
- 连续登录失败锁定、登录/注册/刷新/换 Token 限流（防撞库与批量滥用）

### OAuth2 接入（Integration）

- **授权码模式** `authorization_code`：用户授权 → 一次性 code → 换取用户令牌
- **客户端凭证模式** `client_credentials`：服务间调用，应用令牌
- **UserInfo Endpoint**：用户/应用信息 + 权限规则
- **数据权限接口**：供子系统按需拉取 `data_scopes` 做行级数据过滤

### 安全审计（Audit）

- 全量写操作审计（登录/注册/CRUD/授权/改密等），声明式中间件记录
- 记录操作人、模块、动作、IP、User-Agent、耗时、成败与错误信息
- 批量异步落库，不阻塞业务请求

---

## 🧱 技术栈

| 层 | 技术 |
| --- | --- |
| 后端 | Go 1.25 + [infra-go](https://github.com/chihqiang/infra-go)（conf/logger/orm/redisx/httpx/jwt/ratelimit） |
| 数据库 | GORM，支持 **SQLite / MySQL / PostgreSQL**（默认 SQLite，零依赖开箱即用） |
| 缓存 | Redis（可选，权限集缓存 / 多节点水平扩展） |
| 前端 | Vue 3 + TypeScript + Vite（`ui/` 目录，构建产物经 Go embed 打包进单二进制） |
| 部署 | 多阶段 Docker 构建，纯 Go API 单二进制 |

---

## 🚀 快速开始

### 前置要求

- Go 1.25+

### 方式一：直接运行（默认 SQLite，无需额外依赖）

```bash
go run .
```

服务启动后：

- 健康检查：<http://127.0.0.1:8080/health>
- 内置管理员：`admin` / `admin123`（**生产环境请立即修改**）
- 前端控制台（`ui/` 目录）已通过 Go embed 打包进服务，直接访问 <http://127.0.0.1:8080> 即可打开

### 方式二：Docker 部署

> Docker 镜像使用**独立的配置文件** `config.docker.yaml`（构建时复制进容器并命名为
> `/app/config.yaml`），与本地开发用的 `config.yaml` 完全分离，互不影响。

```bash
docker build -t zhiqiangwang/app:q-iam .

# 基础启动（无数据卷）：SQLite 文件落在容器内 /app/data.db，容器删除后数据丢失，仅适合测试
docker run -d --name q-iam -p 8080:8080 \
  -e JWT_SECRET='MCCZQJbmKNWze5gBIRl+UB3cjrPrzLqqYi091WILxqu8ekjKiAWcBmyC5QNI6rMV' \
  zhiqiangwang/app:q-iam

# 查看日志（-f 持续跟踪输出）
docker logs -f q-iam

# 停止并删除旧的 q-iam 容器
docker rm -f q-iam
```

> `config.docker.yaml` 中的 `DB_DRIVER`（数据库驱动）、`DB_DATABASE`（数据库连接）
> 由 Dockerfile 通过 `ENV` 提供默认值；`JWT_SECRET`（签名密钥）为敏感数据，
> **不固化进镜像**，必须运行时用 `-e` 注入。生产部署示例：

```bash
# 挂载 SQLite 数据卷：-v $(pwd)/data:/app/data，宿主机目录与容器 /app/data 互通。
# 注意：DB_DATABASE 必须指向挂载卷内的路径 ./data/data.db，数据库才会落盘到宿主机，
#       否则数据库写在 /app/data.db（容器根目录），挂载的 data/ 是空目录，数据不持久化。
mkdir -p data
docker run -d --name q-iam -p 8080:8080 \
  -e JWT_SECRET='MCCZQJbmKNWze5gBIRl+UB3cjrPrzLqqYi091WILxqu8ekjKiAWcBmyC5QNI6rMV' \
  -e DB_DRIVER='sqlite' \
  -e DB_DATABASE='./data/data.db' \
  -v $(pwd)/data:/app/data \
  zhiqiangwang/app:q-iam
```

切换 MySQL 示例：`-e DB_DRIVER=mysql -e DB_DATABASE='user:pass@tcp(mysql:3306)/qiam?charset=utf8mb4&parseTime=True&loc=Local'`

---

## ⚙️ 配置说明

配置文件：`config.yaml`（支持环境变量展开，如 `${JWT_SECRET}` 覆盖敏感配置）。

| 配置块 | 说明 | 默认值 |
| --- | --- | --- |
| `server` | HTTP 监听地址与端口 | `0.0.0.0:8080` |
| `db` | 数据库驱动与连接（sqlite/mysql/postgres） | `sqlite:./data.db` |
| `jwt` | 签名密钥、签发者、令牌有效期 | HS256 / 2h / 168h |
| `security.password_policy` | 密码强度策略 | 见 config.yaml 注释 |
| `security.login` | 登录失败锁定阈值与时长 | 5 次 / 15m |
| `security.register.enabled` | 是否开放注册 | `true` |
| `cors` | 跨域白名单（空则 `*`） | `*` |
| `redis` | 权限缓存 Redis 地址（空则不启用） | 空 |
| `pprof.enabled` | 性能分析开关 | `false` |

> SQLite 场景可配置 `PRAGMA busy_timeout`，代码已内置处理写冲突。

### ⚠️ 多实例部署约束（重要）

**单实例部署可完全不配 Redis**（限流/缓存/审计均回退到进程内实现，开箱即用）。
但**多实例横向扩展时必须启用 Redis**，否则安全语义会被静默弱化：

| 组件 | 无 Redis（进程内）时多实例的后果 |
| --- | --- |
| 登录/注册/刷新/换 Token 限流 | 限流按实例独立计算，总量 = 单机阈值 × 节点数，撞库防护被稀释 |
| 权限/账号缓存 | 授权变更只失效本节点缓存，其他节点 TTL 窗口内仍是旧数据 |
| 访问令牌撤销黑名单 | 登出的 token 只在本节点失效，其他节点在剩余 TTL 内仍可用 |
| 审计落库 | 各实例独立攒批写库，无集中视角；实例崩溃丢其内存队列日志 |

服务启动时若未配置 Redis 会打印告警日志。生产多实例部署请配置 `redis`（见 `config.docker.yaml`），
限流/缓存/黑名单/授权码消费会自动切换到 Redis 分布式实现。

---

## 📁 目录结构

```text
q-iam/
├── main.go              # 程序入口：加载配置 → 创建 ServiceContext → 启动服务
├── config.yaml          # 运行配置（本地开发 / 直接运行）
├── config.docker.yaml   # Docker 部署专用配置（构建时复制进容器）
├── config/              # 配置结构定义
├── db/                  # 数据库迁移与种子数据（内置 admin + 系统策略）
├── model/               # 数据模型（q_iam_* 表）
├── logic/               # 业务逻辑层（Service）
├── handler/             # HTTP 处理器
├── middleware/          # 中间件（认证/权限/审计/加载账号）
├── route/               # 路由注册（统一 /api/v1 前缀）
├── svc/                 # 服务上下文（依赖装配）
├── ui/                  # 前端控制台（Vue 3 + TypeScript + Vite）
│   ├── embed.go         # //go:embed all:dist 将构建产物打包进服务二进制
│   ├── src/             # 前端源码（api/components/views/stores 等）
│   └── dist/            # 生产构建产物（npm run build 生成）
└── docs/                # 技术文档
    ├── oauth2-integration.md   # OAuth2 对接指南
    └── permission-design.md    # 权限设计
```

### 分层说明

- **model**：领域实体，GORM 模型，全部以 `q_iam_` 前缀建表
- **logic**：核心业务逻辑，无 HTTP 依赖，便于复用与测试（缓存统一用 infra-go cache：无 Redis 用进程内 MemCache，配置 Redis 后切换 RedisCache）
- **handler**：参数绑定、统一响应（`{code, msg, data}`）、审计调用
- **middleware**：请求链路横切关注点（认证 / 权限 / 审计 / 加载账号 / 信任代理）
- **route**：唯一的路由注册点，声明式声明「审计模块 + 所需权限动作」
- **svc**：服务上下文，集中装配全部依赖，main 只负责启动

---

## 🔌 API 概览

统一前缀 `/api/v1`，响应统一为 `{ "code": 0, "msg": "ok", "data": ... }`。

| 模块 | 方法 / 路径 | 说明 |
| --- | --- | --- |
| 认证 | `POST /auth/login` `POST /auth/register` `POST /auth/refresh` `POST /auth/logout` | 登录/注册/刷新/退出 |
| 认证 | `GET /auth/me` | 当前账号 + 生效权限（前端过滤菜单） |
| 认证 | `PUT /auth/password` | 个人中心修改密码 |
| 认证 | `POST /auth/token` | 应用换取令牌（OAuth2） |
| 认证 | `GET /auth/data-permissions` | 权限规则 + 数据范围 |
| OAuth2 | `GET /oauth/app-info` `POST /oauth/authorize` `GET /oauth/userinfo` | 授权码流程 / 用户信息 |
| 账号 | `GET/POST /accounts`，`GET/PUT/DELETE /accounts/{id}` | 账号 CRUD |
| 账号 | `GET /accounts/all` | 全部启用账号（授权下拉用） |
| 账号 | `PUT /accounts/{id}/password` `/reset-password` | 修改/重置密码 |
| 账号组 | `GET/POST /groups`，`GET/PUT/DELETE /groups/{id}` | 账号组 CRUD |
| 账号组 | `GET /groups/all` | 全部启用账号组（下拉用） |
| 账号组 | `POST/DELETE/PUT /groups/{id}/members` | 成员管理 |
| 策略 | `GET/POST /policies`，`GET/PUT/DELETE /policies/{id}` | 策略 CRUD |
| 策略 | `GET /policies/all` | 全部启用策略（授权选择用） |
| 授权语句 | `GET/POST /statements`，`GET/PUT/DELETE /statements/{id}` | 授权语句池 CRUD（独立菜单，策略只负责关联） |
| 授权语句 | `GET /statements/all` | 全部语句（策略关联选择用） |
| 授权 | `POST/DELETE /grants`，`GET /grants/...` | 策略绑定/解绑/查询 |
| 应用 | `GET/POST /apps`，`GET/PUT/DELETE /apps/{id}` | 应用 CRUD |
| 应用 | `GET /apps/all` | 全部启用应用（下拉用） |
| 应用 | `POST /apps/{id}/reset-secret` | 重置密钥 |
| 审计 | `GET /audit-logs` `GET /audit-logs/modules` | 审计日志查询 |
| 系统 | `POST /cleanup/history` | 清理历史数据 |
| 其他 | `GET /health` | 健康检查 |

> 各接口鉴权要求、请求/响应示例详见 [docs/oauth2-integration.md](./docs/oauth2-integration.md)。
>
> 前端控制台（Web UI）源码位于本仓库 `ui/` 目录，构建产物经 Go embed 打包进服务二进制。

---

## 📚 文档

- [OAuth2 对接指南](./docs/oauth2-integration.md) — 第三方应用接入流程、接口示例、安全要点
- [权限设计](./docs/permission-design.md) — 权限模型、判定语义、数据权限、缓存机制

---

## 🛡️ 安全设计要点

1. **密钥保护**：JWT 密钥支持环境变量注入；应用密钥 AES-256-GCM 加密存储，仅创建/重置时返回一次明文
2. **密码安全**：Bcrypt 存储、强度策略校验、登录失败锁定、密码禁含账号名
3. **权限安全**：显式 Deny 优先、系统策略不可改删、内置 admin 全权限放行；策略语句支持 `Resource` 资源维度（管理接口仅全资源 `*` 规则生效，带资源限定的规则供子系统经 `/auth/data-permissions` 做资源级判定）
4. **OAuth2 安全**：回调地址精确匹配（防开放重定向）、`state` 防 CSRF、授权码一次性（防重放）、跨应用隔离
5. **审计留痕**：全量写操作审计（权限校验失败同样记录），批量落库失败自动重试，避免瞬时抖动丢日志
6. **限流防护**：登录/注册/刷新/换 Token 令牌桶限流（可选 Redis 分布式），防撞库与批量滥用
7. **会话控制**：访问令牌携带唯一 jti，`POST /auth/logout` 同时吊销当前 access token（加入黑名单，立即失效，不再依赖自然过期）；刷新令牌重用连坐带时间窗缓冲（短时间内连续多次才吊销全部，避免弱网重试误伤多设备会话）
8. **数据权限落地**：策略 `DataScope` 除供子系统经 `/auth/data-permissions` 拉取外，账号管理列表/下拉亦按其过滤（`self` 仅本人、`group` 仅组内，`attribute` 保守降级为仅本人），防止越权查看

---

## 🧪 本地开发

```bash
# 后端（本仓库）
go run .                        # 启动（默认 8080）

go build ./... && go vet ./...  # 质量检查
go test ./...                   # 运行测试
```

前端控制台源码位于本仓库 `ui/` 目录：

```bash
cd ui
npm install
npm run dev                     # Vite 5173，/api 代理到 127.0.0.1:8080
npm run build                   # 生产构建到 ui/dist（后端 Go embed 自动打包）
npm run typecheck               # TS 类型检查
```

> `ui/dist` 构建产物由 `ui/embed.go` 通过 `//go:embed all:dist` 打进 Go 二进制；重新 `npm run build` 后需重新编译后端才生效。

---
