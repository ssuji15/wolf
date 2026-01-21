package util

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func LoadSeccomp(path string) (*specs.LinuxSeccomp, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var seccomp specs.LinuxSeccomp
	if err := json.Unmarshal(b, &seccomp); err != nil {
		return nil, err
	}
	return &seccomp, nil
}

func GetCodePath(codeHash string) string {
	return fmt.Sprintf("jobs/code/%s", codeHash)
}

func GetOutputPath(outputHash string) string {
	return fmt.Sprintf("jobs/output/%s.log", outputHash)
}

func RecordSpanError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func GetCodeKey(codeHash string) string {
	return fmt.Sprintf("code:%s", codeHash)
}

func GetOutputHashKey(codeHash string) string {
	return fmt.Sprintf("outputHash:%s", codeHash)
}

func GetIdempotencyKey(key string) string {
	return fmt.Sprintf("idempotency:%s", key)
}
