package route

import (
	"net/http"
	"net/http/pprof"

	"github.com/chihqiang/ten-go/rest"
)

// pprofRoutes 注册标准 net/http/pprof 处理器；ten-go rest 未内置该辅助函数。
func pprofRoutes(prefix string) []rest.Route {
	if prefix == "" {
		prefix = "/debug/pprof"
	}
	routes := []rest.Route{
		{Method: http.MethodGet, Path: prefix + "/", Handler: pprof.Index},
		{Method: http.MethodGet, Path: prefix + "/cmdline", Handler: pprof.Cmdline},
		{Method: http.MethodGet, Path: prefix + "/profile", Handler: pprof.Profile},
		{Method: http.MethodGet, Path: prefix + "/symbol", Handler: pprof.Symbol},
		{Method: http.MethodGet, Path: prefix + "/trace", Handler: pprof.Trace},
	}
	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		routes = append(routes, rest.Route{
			Method:  http.MethodGet,
			Path:    prefix + "/" + name,
			Handler: pprof.Handler(name).ServeHTTP,
		})
	}
	return routes
}
