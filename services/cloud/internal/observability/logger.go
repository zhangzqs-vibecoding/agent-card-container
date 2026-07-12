package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
)

func NewJSONLogger(writer io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attribute slog.Attr) slog.Attr {
			switch attribute.Key {
			case slog.TimeKey:
				attribute.Key = "timestamp"
			case slog.MessageKey:
				attribute.Key = "event"
			}
			return attribute
		},
	})
	return slog.New(handler)
}

type requestMetadataKey struct{}

type RequestMetadata struct {
	mutex     sync.RWMutex
	requestID string
	userID    string
}

type RequestMetadataSnapshot struct {
	RequestID string
	UserID    string
}

func NewRequestMetadata(requestID string) *RequestMetadata {
	return &RequestMetadata{requestID: requestID}
}

func WithRequestMetadata(ctx context.Context, metadata *RequestMetadata) context.Context {
	return context.WithValue(ctx, requestMetadataKey{}, metadata)
}

func SetAuthenticatedUser(ctx context.Context, userID string) {
	metadata, ok := ctx.Value(requestMetadataKey{}).(*RequestMetadata)
	if !ok || metadata == nil {
		return
	}
	metadata.mutex.Lock()
	defer metadata.mutex.Unlock()
	metadata.userID = userID
}

func RequestSnapshot(ctx context.Context) RequestMetadataSnapshot {
	metadata, ok := ctx.Value(requestMetadataKey{}).(*RequestMetadata)
	if !ok || metadata == nil {
		return RequestMetadataSnapshot{}
	}
	metadata.mutex.RLock()
	defer metadata.mutex.RUnlock()
	return RequestMetadataSnapshot{
		RequestID: metadata.requestID,
		UserID:    metadata.userID,
	}
}

func ErrorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "internal"
	}
}
