package telemetry

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"opencode2api/internal/protocol"
)

func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				if value == http.ErrAbortHandler {
					panic(value)
				}
				var traceID uint64
				var requestID string
				if meta := MetaFromRequest(r); meta != nil {
					traceID, requestID = meta.TraceID, meta.Request
					meta.Outcome = "handler_panic"
				}
				logger.Error("request handler panicked", "component", "http", "event", "request_panic",
					"trace_id", traceID, "request_id", requestID, "method", r.Method, "path", r.URL.Path, "error", fmt.Sprint(value), "stack", string(debug.Stack()))
				protocol.WriteError(w, protocol.Chat, http.StatusInternalServerError, "internal server error", "server_error", "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
