package handler

import (
	"net/http"

	"github.com/chihqiang/ten-go/core/cast"
	"github.com/chihqiang/ten-go/rest"
)

// pathValue 读取路径参数并转换为 T；参数缺失、为空或转换失败时返回 def。
func pathValue[T any](r *http.Request, key string, def ...T) T {
	return valueOr(rest.PathParam(r, key), def)
}

// queryValue 读取查询参数并转换为 T；参数缺失、为空或转换失败时返回 def。
func queryValue[T any](r *http.Request, key string, def ...T) T {
	return valueOr(r.URL.Query().Get(key), def)
}

func valueOr[T any](raw string, def []T) T {
	if raw == "" {
		return defaultValue(def)
	}
	v, err := cast.ToE[T](raw)
	if err != nil {
		return defaultValue(def)
	}
	return v
}

func defaultValue[T any](def []T) T {
	var zero T
	if len(def) > 0 {
		return def[0]
	}
	return zero
}
