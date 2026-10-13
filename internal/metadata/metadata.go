package metadata

import (
	"fmt"
	"os"
	"strings"
)

type Metadata struct {
	ContextFiles []string
	UserPrompt   string
}

func Load(path string) (Metadata, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("metadata %q: %w", path, err)
	}
	meta, err := Parse(content)
	if err != nil {
		return Metadata{}, fmt.Errorf("metadata %q: %w", path, err)
	}
	return meta, nil
}

func Parse(data []byte) (Metadata, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var meta Metadata
	seen := map[string]bool{}
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if isBlankOrComment(line) {
			continue
		}
		keyLine := index + 1
		if isIndented(line) {
			return Metadata{}, fmt.Errorf("line %d: indented line outside a key", keyLine)
		}
		key, inline, found := strings.Cut(line, ":")
		if !found {
			return Metadata{}, fmt.Errorf("line %d: expected key: value", keyLine)
		}
		if key != "context_files" && key != "user_prompt" {
			return Metadata{}, fmt.Errorf("line %d: unknown key %q", keyLine, key)
		}
		if seen[key] {
			return Metadata{}, fmt.Errorf("line %d: duplicate key %q", keyLine, key)
		}
		seen[key] = true

		bodyEnd := index + 1
		for bodyEnd < len(lines) && (isBlank(lines[bodyEnd]) || isIndented(lines[bodyEnd])) {
			bodyEnd++
		}
		body := lines[index+1 : bodyEnd]
		inline = inlineValue(inline)

		var err error
		switch key {
		case "context_files":
			meta.ContextFiles, err = readContextFiles(inline, body, keyLine)
		case "user_prompt":
			meta.UserPrompt, err = readUserPrompt(inline, body, keyLine)
		}
		if err != nil {
			return Metadata{}, err
		}
		index = bodyEnd - 1
	}
	for _, key := range []string{"context_files", "user_prompt"} {
		if !seen[key] {
			return Metadata{}, fmt.Errorf("missing key %q", key)
		}
	}
	return meta, nil
}

func readContextFiles(inline string, body []string, keyLine int) ([]string, error) {
	if inline != "" {
		return nil, fmt.Errorf("line %d: context_files takes its items on the lines below", keyLine)
	}
	var items []string
	for offset, line := range body {
		if isBlankOrComment(line) {
			continue
		}
		value, isItem := strings.CutPrefix(strings.TrimLeft(line, " "), "- ")
		value = strings.TrimSpace(value)
		if !isItem || value == "" {
			return nil, fmt.Errorf("line %d: expected a list item \"- path\"", keyLine+1+offset)
		}
		items = append(items, value)
	}
	return items, nil
}

func readUserPrompt(inline string, body []string, keyLine int) (string, error) {
	for offset, line := range body {
		if !isBlankOrComment(line) {
			return "", fmt.Errorf("line %d: unexpected text under user_prompt", keyLine+1+offset)
		}
	}
	if inline == "" {
		return "", fmt.Errorf("line %d: user_prompt needs a value", keyLine)
	}
	return inline, nil
}

func inlineValue(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "#") {
		return ""
	}
	return value
}

func isBlank(line string) bool {
	return strings.TrimSpace(line) == ""
}

func isBlankOrComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
