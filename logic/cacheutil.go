package logic

import (
	"strconv"
)

// cacheGetString 将 cache.Cache.Get 返回的 any 值转为字符串。
// 兼容两种后端：
//   - MemCache：直接存储原始类型（SetEx 写入 string，返回 string）；
//   - RedisCache：值经 JSON 序列化，string 反序列化后仍为 string。
//
// 未命中或值类型不符时返回 ("", false)。未命中需调用方结合 errors.Is(err, cache.ErrNotFound) 判断。
func cacheGetString(v any, err error) (string, bool) {
	if err != nil || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// cacheGetInt64 将 cache.Cache.Get 返回的 any 值转为 int64。
// 兼容两种后端：
//   - MemCache：Increment 存入 int64，返回 int64；
//   - RedisCache：int64 经 JSON 序列化后反序列化为 float64。
//
// 未命中或值类型不符时返回 (0, false)。未命中需调用方结合 errors.Is(err, cache.ErrNotFound) 判断。
func cacheGetInt64(v any, err error) (int64, bool) {
	if err != nil || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case string:
		i, e := strconv.ParseInt(n, 10, 64)
		if e != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}
