// This file is generated with go generate. Any changes to it will be lost after
// subsequent generates.
// If you want to edit it go to types.go.template

package handler

import (
	"net/http"

	"github.com/ironsmile/nedomi/config"

	"github.com/ironsmile/nedomi/handler/cache"
	"github.com/ironsmile/nedomi/handler/dir"
	"github.com/ironsmile/nedomi/handler/flv"
	"github.com/ironsmile/nedomi/handler/headers"
	"github.com/ironsmile/nedomi/handler/mp4"
	"github.com/ironsmile/nedomi/handler/pprof"
	"github.com/ironsmile/nedomi/handler/proxy"
	"github.com/ironsmile/nedomi/handler/purge"
	"github.com/ironsmile/nedomi/handler/status"
	"github.com/ironsmile/nedomi/handler/throttle"
	"github.com/ironsmile/nedomi/types"
)

var handlerTypes = map[string]newHandlerFunc{

	"cache": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return cache.New(cfg, l, next)
	},

	"dir": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return dir.New(cfg, l, next)
	},

	"flv": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return flv.New(cfg, l, next)
	},

	"headers": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return headers.New(cfg, l, next)
	},

	"mp4": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return mp4.New(cfg, l, next)
	},

	"pprof": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return pprof.New(cfg, l, next)
	},

	"proxy": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return proxy.New(cfg, l, next)
	},

	"purge": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return purge.New(cfg, l, next)
	},

	"status": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return status.New(cfg, l, next)
	},

	"throttle": func(cfg *config.Handler, l *types.Location, next http.Handler) (http.Handler, error) {
		return throttle.New(cfg, l, next)
	},
}
