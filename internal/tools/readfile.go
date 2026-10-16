package tools

import (
	"fmt"
	"os"
)

func ReadFileToolSpec() string {
	return `
readfile()
`
}

// ReadFile returns the contents of the file at path as a string.
func ReadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return string(content), nil
}
