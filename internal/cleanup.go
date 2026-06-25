package internal

import (
	"os"
	"path/filepath"
)

func Cleanup() {
	tempDir := os.TempDir()
	pattern := filepath.Join(tempDir, "py-cli-*.py")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
}
