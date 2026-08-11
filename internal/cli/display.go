package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// display shortens a path for output: relative to the working directory when it
// lives there, otherwise abbreviating the home directory to "~".
func display(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}

	if wd, err := os.Getwd(); err == nil {
		if rel, ok := within(wd, path); ok {
			return rel
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, ok := within(home, path); ok {
			return "~/" + rel
		}
	}
	return path
}

// within reports path relative to base, when it is inside base.
func within(base, path string) (string, bool) {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
