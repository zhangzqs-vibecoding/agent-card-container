package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
)

type AccessLogConfig struct {
	Logger       *slog.Logger
	NewRequestID func() string
	Now          func() time.Time
}

func AccessLogMiddleware(config AccessLogConfig) func(http.Handler) http.Handler {
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	newRequestID := config.NewRequestID
	if newRequestID == nil {
		newRequestID = func() string { return "request-unavailable" }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			startedAt := now()
			requestID := newRequestID()
			metadata := observability.NewRequestMetadata(requestID)
			ctx := observability.WithRequestMetadata(request.Context(), metadata)
			request = request.WithContext(ctx)
			writer.Header().Set("X-Request-ID", requestID)
			captured := &accessLogResponseWriter{ResponseWriter: writer, status: http.StatusOK}
			next.ServeHTTP(captured, request)

			snapshot := observability.RequestSnapshot(ctx)
			attributes := []any{
				"requestId", snapshot.RequestID,
				"method", request.Method,
				"path", request.URL.Path,
				"status", captured.status,
				"durationMs", max(0, now().Sub(startedAt).Milliseconds()),
				"remoteIp", remoteIP(request.RemoteAddr),
			}
			if snapshot.UserID != "" {
				attributes = append(attributes, "userId", snapshot.UserID)
			}
			logger.InfoContext(context.WithoutCancel(ctx), "http_request", attributes...)
		})
	}
}

type accessLogResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (writer *accessLogResponseWriter) WriteHeader(status int) {
	if writer.wroteHeader {
		return
	}
	writer.status = status
	writer.wroteHeader = true
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *accessLogResponseWriter) Write(content []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(content)
}

func (writer *accessLogResponseWriter) Flush() {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (writer *accessLogResponseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func remoteIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil {
		return host
	}
	return remoteAddress
}
