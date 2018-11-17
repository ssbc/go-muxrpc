package logging

import (
	"context"
	"net/http"

	kitlog "github.com/go-kit/kit/log"
)

type logctxKeyT string

var LogCTXKey logctxKeyT = "loggingContextKey"

func NewContext(ctx context.Context, log Interface) context.Context {
	return context.WithValue(ctx, LogCTXKey, log)
}

func FromContext(ctx context.Context) Interface {
	v, ok := ctx.Value(LogCTXKey).(Interface)
	if !ok {
		return nil
	}
	return v
}

func InjectHandler(mainLog Interface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			if l := FromContext(ctx); l == nil {
				l = kitlog.With(mainLog, "urlPath", req.URL.Path)
				req = req.WithContext(NewContext(ctx, l))
			}
			next.ServeHTTP(w, req)
		})
	}
}
