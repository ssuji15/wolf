package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
)

func TestLoadSeccomp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(tmp string) string
		wantError  bool
		verifyFunc func(*testing.T, *specs.LinuxSeccomp)
	}{
		{
			name: "valid seccomp JSON",
			setup: func(tmp string) string {
				path := filepath.Join(tmp, "seccomp.json")
				data := specs.LinuxSeccomp{DefaultAction: specs.ActErrno}
				b, _ := json.Marshal(data)
				require.NoError(t, os.WriteFile(path, b, 0644))
				return path
			},
			wantError: false,
			verifyFunc: func(t *testing.T, seccomp *specs.LinuxSeccomp) {
				require.Equal(t, specs.ActErrno, seccomp.DefaultAction)
			},
		},
		{
			name: "invalid JSON",
			setup: func(tmp string) string {
				path := filepath.Join(tmp, "invalid.json")
				require.NoError(t, os.WriteFile(path, []byte("{invalid json}"), 0644))
				return path
			},
			wantError: true,
		},
		{
			name: "file does not exist",
			setup: func(tmp string) string {
				return filepath.Join(tmp, "missing.json")
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()
			path := tt.setup(tmp)

			got, err := LoadSeccomp(path)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.verifyFunc != nil {
					tt.verifyFunc(t, got)
				}
			}
		})
	}
}

func TestGetCodePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		codeHash string
		want     string
	}{
		{"simple hash", "abc123", "jobs/code/abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, GetCodePath(tt.codeHash))
		})
	}
}

func TestGetOutputPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputHash string
		want       string
	}{
		{"simple hash", "xyz789", "jobs/output/xyz789.log"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, GetOutputPath(tt.outputHash))
		})
	}
}

func TestGetCodeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		codeHash string
		want     string
	}{
		{"normal", "abc123", "code:abc123"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, GetCodeKey(tt.codeHash))
		})
	}
}

func TestGetOutputHashKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		codeHash string
		want     string
	}{
		{"normal", "abc123", "outputHash:abc123"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, GetOutputHashKey(tt.codeHash))
		})
	}
}
