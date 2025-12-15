package logger

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

type ctxKey struct{}

func Init(serviceName string) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	Log = zerolog.New(os.Stdout).
		With().
		Timestamp().
		Str("service", serviceName).
		Logger()
}

func WithContext(ctx context.Context, log zerolog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, log)
}

func FromContext(ctx context.Context) zerolog.Logger {
	if log, ok := ctx.Value(ctxKey{}).(zerolog.Logger); ok {
		return log
	}
	return Log
}
