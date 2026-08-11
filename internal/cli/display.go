package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// display shortens a path for output, abbreviating the home directory to "~".
func display(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filepath.Join("~", rel))
	}
	return path
}
