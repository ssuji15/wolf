package util

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func EnsureDir(dir string) error {
	if stat, err := os.Stat(dir); err == nil {
		if !stat.IsDir() {
			return fmt.Errorf("uds: path exists but is not a directory: %s", dir)
		}
		return nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("uds: failed to create dir %s: %w", dir, err)
	}
	return nil
}

func RemoveIfExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		// Remove stale socket
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("uds: failed to remove stale socket %s: %w", path, err)
		}
	}
	return nil
}

func VerifyPath(path string) error {
	dir := filepath.Dir(path)

	// Ensure parent directory exists
	if err := EnsureDir(dir); err != nil {
		return err
	}

	// Remove stale socket file if present
	if err := RemoveIfExists(path); err != nil {
		return err
	}

	return nil
}

// Exists checks if a socket file exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsSocketFile checks if a file is a AF_UNIX socket.
func IsSocketFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if info.Mode()&os.ModeSocket != 0 {
		return true, nil
	}

	// Fallback check via syscall
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return false, err
	}

	return stat.Mode&syscall.S_IFSOCK != 0, nil
}
