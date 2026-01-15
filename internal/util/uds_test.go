package util

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ssuji15/wolf/model"
	"github.com/stretchr/testify/require"
)

func TestEnsureDirExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(tmp string) string
		wantError bool
	}{
		{
			name: "directory does not exist",
			setup: func(tmp string) string {
				return filepath.Join(tmp, "newdir")
			},
			wantError: false,
		},
		{
			name: "directory already exists",
			setup: func(tmp string) string {
				dir := filepath.Join(tmp, "existing")
				require.NoError(t, os.Mkdir(dir, 0755))
				return dir
			},
			wantError: false,
		},
		{
			name: "path exists but is a file",
			setup: func(tmp string) string {
				path := filepath.Join(tmp, "file")
				require.NoError(t, os.WriteFile(path, []byte("data"), 0644))
				return path
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()
			path := tt.setup(tmp)

			err := EnsureDirExist(path)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRemoveFileIfExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(tmp string) string
		wantExist bool
	}{
		{
			name: "file exists and is removed",
			setup: func(tmp string) string {
				path := filepath.Join(tmp, "file")
				require.NoError(t, os.WriteFile(path, []byte("data"), 0644))
				return path
			},
			wantExist: false,
		},
		{
			name: "file does not exist",
			setup: func(tmp string) string {
				return filepath.Join(tmp, "missing")
			},
			wantExist: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()
			path := tt.setup(tmp)

			require.NoError(t, RemoveFileIfExists(path))
			require.Equal(t, tt.wantExist, Exists(path))
		})
	}
}

func TestVerifyFileDoesNotExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(tmp string) string
		wantExist bool
	}{
		{
			name: "file exists and is removed",
			setup: func(tmp string) string {
				dir := filepath.Join(tmp, "dir")
				require.NoError(t, os.MkdirAll(dir, 0755))
				path := filepath.Join(dir, "file")
				require.NoError(t, os.WriteFile(path, []byte("data"), 0644))
				return path
			},
			wantExist: false,
		},
		{
			name: "file does not exist but directory does",
			setup: func(tmp string) string {
				dir := filepath.Join(tmp, "dir")
				require.NoError(t, os.MkdirAll(dir, 0755))
				return filepath.Join(dir, "file")
			},
			wantExist: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()
			path := tt.setup(tmp)

			require.NoError(t, VerifyFileDoesNotExist(path))
			require.Equal(t, tt.wantExist, Exists(path))
		})
	}
}

func TestExists(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	existing := filepath.Join(tmp, "file")
	require.NoError(t, os.WriteFile(existing, []byte("data"), 0644))

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"existing file", existing, true},
		{"missing file", filepath.Join(tmp, "missing"), false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, Exists(tt.path))
		})
	}
}

func TestIsSocketFile_NonSocket(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported on windows")
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "file")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))

	isSocket, err := IsSocketFile(path)
	require.NoError(t, err)
	require.False(t, isSocket)
}

func TestDispatchJob_InvalidSocket(t *testing.T) {
	t.Parallel()

	job := &model.Job{
		ExecutionEngine: "test-engine",
	}

	err := DispatchJob(context.Background(), "/non/existent/socket", job, []byte("code"))
	require.Error(t, err)
}
