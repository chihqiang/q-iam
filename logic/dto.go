package logic

// PageRequest 通用分页请求（query 参数绑定）。
// 各模块列表请求通过匿名内嵌复用；infra-go 的 form 绑定与校验会递归展开 embedded 字段，
// 因此 `page`/`size` 参数与 `required,min=1` 校验行为与原先一致。
type PageRequest struct {
	Page int `form:"page" binding:"required,min=1"`
	Size int `form:"size" binding:"required,min=1,max=100"`
}

// PageResponse 通用分页响应（{data, total}，与前端 Paginated<T> 契约一致）。
type PageResponse[T any] struct {
	Data  []T   `json:"data"`
	Total int64 `json:"total"`
}

// uniqueIDs 去重整数切片并保持原有相对顺序。
func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
