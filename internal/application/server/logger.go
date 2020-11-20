package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/middleware"
	"go.uber.org/zap"
)

type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
}

// middlewareLogger is used for request logging. Only Zap logger is supported now, or dummy.
func middlewareLogger(logger Logger) func(next http.Handler) http.Handler {
	l, ok := logger.(*zap.SugaredLogger)
	if ok {
		log := l.Desugar()
		return func(next http.Handler) http.Handler {
			fn := func(w http.ResponseWriter, r *http.Request) {
				ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

				t := time.Now()
				defer func() {
					// Do not log kube-probe healtchecks
					if !strings.HasPrefix(r.UserAgent(), "kube-probe") {
						log.Info("Served",
							zap.Any("metadata", map[string]interface{}{
								"request-headers": map[string]interface{}{
									"Content-Type":    r.Header.Get("Content-Type"),
									"Content-Length":  r.Header.Get("Content-Length"),
									"User-Agent":      r.UserAgent(),
									"Server":          r.Header.Get("Server"),
									"Via":             r.Header.Get("Via"),
									"Accept":          r.Header.Get("Accept"),
									"X-FORWARDED-FOR": r.Header.Get("X-FORWARDED-FOR"),
								},
							}),
							// Essentials
							zap.String("method", r.Method),
							zap.String("RemoteAddr", r.RemoteAddr),
							zap.String("Proto", r.Proto),
							zap.String("Path", r.URL.Path),
							zap.String("reqID", middleware.GetReqID(r.Context())),
							zap.Duration("Duration", time.Since(t)),
							zap.Int("size", ww.BytesWritten()),
							zap.Int("status", ww.Status()),
						)
					}
				}()

				next.ServeHTTP(ww, r)
			}
			return http.HandlerFunc(fn)
		}
	}
	// if not zap.SugaredLogger, return dummy middleware
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
	}
}
