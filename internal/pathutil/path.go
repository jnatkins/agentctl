package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

func Expand(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func ResolveRelative(base, path string) string {
	if path == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "~/") {
		return Expand(path)
	}
	return filepath.Join(base, path)
}
