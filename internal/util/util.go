package util

import (
	"encoding/json"
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
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
