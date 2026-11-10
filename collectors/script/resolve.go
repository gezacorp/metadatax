package script

import (
	"os"
	"path/filepath"
	"strings"
)

const maxLinks = 40

// resolveIn resolves the symlinks of path within root, absolute link targets
// included.
func resolveIn(root, path string) string {
	parts := strings.Split(filepath.Clean(path), "/")
	resolved := "/"

	for links := 0; len(parts) > 0; {
		p := parts[0]
		parts = parts[1:]

		switch p {
		case "", ".":
			continue
		case "..":
			resolved = filepath.Dir(resolved)

			continue
		}

		next := filepath.Join(resolved, p)

		target, err := os.Readlink(filepath.Join(root, next))
		if err != nil {
			resolved = next

			continue
		}

		if links++; links > maxLinks {
			return path
		}

		if filepath.IsAbs(target) {
			resolved = "/"
		}

		parts = append(strings.Split(target, "/"), parts...)
	}

	return resolved
}
