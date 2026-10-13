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
		for bodyEnd < len(lines) && continuesKey(lines, bodyEnd) {
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
	if strings.HasPrefix(inline, "[") {
		return nil, fmt.Errorf("line %d: flow lists aren't supported, put each file on its own \"- path\" line", keyLine)
	}
	if inline != "" {
		return nil, fmt.Errorf("line %d: context_files takes its items on the lines below", keyLine)
	}
	var items []string
	seen := map[string]bool{}
	var indent string
	for offset, line := range body {
		if isBlankOrComment(line) {
			continue
		}
		lineNumber := keyLine + 1 + offset
		rest := strings.TrimLeft(line, " ")
		if strings.HasPrefix(rest, "\t") || strings.HasPrefix(rest, "-\t") {
			return nil, fmt.Errorf("line %d: use spaces, not tabs", lineNumber)
		}
		lineIndent := line[:len(line)-len(rest)]
		item, isItem := strings.CutPrefix(rest, "-")
		if !isItem || (item != "" && item[0] != ' ') {
			return nil, fmt.Errorf("line %d: expected a list item \"- path\"", lineNumber)
		}
		if len(items) == 0 {
			indent = lineIndent
		} else if lineIndent != indent {
			return nil, fmt.Errorf("line %d: items must use the same indent", lineNumber)
		}
		path, err := contextFilePath(strings.TrimSpace(item), lineNumber)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			return nil, fmt.Errorf("line %d: duplicate context file %q", lineNumber, path)
		}
		seen[path] = true
		items = append(items, path)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("line %d: context_files needs at least one item", keyLine)
	}
	return items, nil
}

func contextFilePath(raw string, lineNumber int) (string, error) {
	var path string
	if raw != "" && (raw[0] == '"' || raw[0] == '\'') {
		end := strings.IndexByte(raw[1:], raw[0])
		if end < 0 {
			return "", fmt.Errorf("line %d: unclosed quote", lineNumber)
		}
		path = raw[1 : end+1]
		rest := raw[end+2:]
		trimmed := strings.TrimLeft(rest, " \t")
		if trimmed != "" && (len(trimmed) == len(rest) || trimmed[0] != '#') {
			return "", fmt.Errorf("line %d: unexpected text after quoted path", lineNumber)
		}
	} else {
		if raw == "-" || strings.HasPrefix(raw, "- ") || strings.HasPrefix(raw, "-\t") {
			return "", fmt.Errorf("line %d: nested lists aren't supported", lineNumber)
		}
		if err := checkPlainValue(raw, pathSpecialCharacters, lineNumber); err != nil {
			return "", err
		}
		path = strings.TrimSpace(stripComment(raw))
	}
	if path == "" {
		return "", fmt.Errorf("line %d: list item needs a path", lineNumber)
	}
	return path, nil
}

func stripComment(value string) string {
	for index := 0; index < len(value); index++ {
		if value[index] == '#' && (index == 0 || value[index-1] == ' ' || value[index-1] == '\t') {
			return value[:index]
		}
	}
	return value
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
	if err := checkPlainValue(inline, promptSpecialCharacters, keyLine); err != nil {
		return "", err
	}
	return inline, nil
}

const promptSpecialCharacters = "[{*&!%@`"

const pathSpecialCharacters = promptSpecialCharacters + "|>?"

func checkPlainValue(value string, specialCharacters string, lineNumber int) error {
	if value != "" && strings.IndexByte(specialCharacters, value[0]) >= 0 {
		return fmt.Errorf("line %d: a value can't start with %q, wrap it in quotes", lineNumber, value[:1])
	}
	return nil
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

func belongsToKey(line string) bool {
	return isBlank(line) || isIndented(line) || strings.HasPrefix(line, "-")
}

func continuesKey(lines []string, at int) bool {
	if belongsToKey(lines[at]) {
		return true
	}
	if !strings.HasPrefix(lines[at], "#") {
		return false
	}
	for next := at + 1; next < len(lines); next++ {
		if !isBlankOrComment(lines[next]) {
			return belongsToKey(lines[next])
		}
	}
	return false
}
