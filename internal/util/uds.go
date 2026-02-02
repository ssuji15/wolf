package util

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
)

func EnsureDirExist(dir string) error {
	defer ChOwn(dir, 1000)
	if stat, err := os.Stat(dir); err == nil {
		if !stat.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", dir)
		}
		return nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create dir %s: %w", dir, err)
	}
	return nil
}

func RemoveFileIfExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove path %s: %w", path, err)
		}
	}
	return nil
}

func VerifyFileDoesNotExist(path string) error {
	dir := filepath.Dir(path)

	// Ensure parent directory exists
	if err := EnsureDirExist(dir); err != nil {
		return err
	}

	// Remove file if present
	if err := RemoveFileIfExists(path); err != nil {
		return err
	}

	return nil
}

// Exists checks if a file exists.
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
	return (stat.Mode & syscall.S_IFMT) == syscall.S_IFSOCK, nil
}
func ChOwn(dir string, id int) error {
	if err := os.Chown(dir, id, id); err != nil {
		log.Printf("Chown failed: %v", err)
		return err
	}
	if err := os.Chmod(dir, 0755); err != nil {
		return err
	}
	return nil
}
