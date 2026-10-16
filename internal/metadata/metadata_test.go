package metadata

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadSample(t *testing.T) {
	got, err := Load("testdata/metadata.yaml")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Metadata{
		ContextFiles: []string{"testdata/claude_hooks.md", "docs/openrouter-agent-loop.md"},
		UserPrompt:   "Explain how Claude Code hooks work.\nKeep it under 200 words.\n",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load(%q) error = nil, want error", path)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("Load() error = %q, want it to contain %q", err, path)
	}
}

func TestLoadBadContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.yaml")
	content := "context_files:\n  - a.md\nmodel: x\nuser_prompt: hi\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load(%q) error = nil, want error", path)
	}
	for _, want := range []string{path, "line 3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Load() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Metadata
		wantErr string
	}{
		{
			name:  "keys in order",
			input: "context_files:\n  - a.md\n  - b.md\nuser_prompt: Explain hooks\n",
			want:  Metadata{ContextFiles: []string{"a.md", "b.md"}, UserPrompt: "Explain hooks"},
		},
		{
			name:  "keys reversed",
			input: "user_prompt: Explain hooks\ncontext_files:\n  - a.md\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "Explain hooks"},
		},
		{
			name:  "blank lines and comments are ignored",
			input: "# top comment\n\ncontext_files:\n  # list note\n  - a.md\n\n# between keys\nuser_prompt: Explain hooks\n\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "Explain hooks"},
		},
		{
			name:  "crlf line endings",
			input: "context_files:\r\n  - a.md\r\nuser_prompt: Explain hooks\r\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "Explain hooks"},
		},
		{
			name:    "missing context_files",
			input:   "user_prompt: Explain hooks\n",
			wantErr: `missing key "context_files"`,
		},
		{
			name:    "missing user_prompt",
			input:   "context_files:\n  - a.md\n",
			wantErr: `missing key "user_prompt"`,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: `missing key "context_files"`,
		},
		{
			name:    "unknown key",
			input:   "context_files:\n  - a.md\nmodel: x\nuser_prompt: hi\n",
			wantErr: `line 3: unknown key "model"`,
		},
		{
			name:    "duplicate key",
			input:   "context_files:\n  - a.md\nuser_prompt: one\nuser_prompt: two\n",
			wantErr: `line 4: duplicate key "user_prompt"`,
		},
		{
			name:    "indented line before any key",
			input:   "  - a.md\ncontext_files:\n  - b.md\nuser_prompt: hi\n",
			wantErr: "line 1:",
		},
		{
			name:    "line without a colon",
			input:   "context_files\n",
			wantErr: "line 1:",
		},
		{
			name:    "inline value for context_files",
			input:   "context_files: a.md\nuser_prompt: hi\n",
			wantErr: "line 1:",
		},
		{
			name:    "non-item line under context_files",
			input:   "context_files:\n  foo\nuser_prompt: hi\n",
			wantErr: "line 2:",
		},
		{
			name:    "text under user_prompt",
			input:   "context_files:\n  - a.md\nuser_prompt: hi\n  more\n",
			wantErr: "line 4:",
		},
		{
			name:    "empty user_prompt",
			input:   "context_files:\n  - a.md\nuser_prompt:\n",
			wantErr: "line 3:",
		},
		{
			name:  "items at column zero",
			input: "context_files:\n- a.md\n- b.md\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a.md", "b.md"}, UserPrompt: "hi"},
		},
		{
			name:  "items indented four spaces",
			input: "context_files:\n    - a.md\n    - b.md\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a.md", "b.md"}, UserPrompt: "hi"},
		},
		{
			name:    "mixed item indents",
			input:   "context_files:\n  - a.md\n    - b.md\nuser_prompt: hi\n",
			wantErr: "line 3: items must use the same indent",
		},
		{
			name:  "trailing comment stripped",
			input: "context_files:\n  - a.md # note\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "hi"},
		},
		{
			name:  "hash without a space is part of the path",
			input: "context_files:\n  - a#b.md\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a#b.md"}, UserPrompt: "hi"},
		},
		{
			name:  "double quoted path with space",
			input: "context_files:\n  - \"a b.md\"\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a b.md"}, UserPrompt: "hi"},
		},
		{
			name:  "single quoted path",
			input: "context_files:\n  - 'a.md'\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "hi"},
		},
		{
			name:  "quoted path with trailing comment",
			input: "context_files:\n  - \"a b.md\" # note\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a b.md"}, UserPrompt: "hi"},
		},
		{
			name:    "unclosed double quote",
			input:   "context_files:\n  - \"a.md\nuser_prompt: hi\n",
			wantErr: "line 2: unclosed quote",
		},
		{
			name:    "unclosed single quote",
			input:   "context_files:\n  - 'a.md\nuser_prompt: hi\n",
			wantErr: "line 2: unclosed quote",
		},
		{
			name:    "text after closing quote",
			input:   "context_files:\n  - \"a.md\" x\nuser_prompt: hi\n",
			wantErr: "line 2: unexpected text after quoted value",
		},
		{
			name:    "text glued to closing quote",
			input:   "context_files:\n  - \"a.md\"#x\nuser_prompt: hi\n",
			wantErr: "line 2: unexpected text after quoted value",
		},
		{
			name:    "empty item with trailing space",
			input:   "context_files:\n  - \nuser_prompt: hi\n",
			wantErr: "line 2: list item needs a path",
		},
		{
			name:    "empty item with only a dash",
			input:   "context_files:\n  -\nuser_prompt: hi\n",
			wantErr: "line 2: list item needs a path",
		},
		{
			name:    "item that is only a comment",
			input:   "context_files:\n  - # note\nuser_prompt: hi\n",
			wantErr: "line 2: list item needs a path",
		},
		{
			name:    "tab before dash",
			input:   "context_files:\n\t- a.md\nuser_prompt: hi\n",
			wantErr: "line 2: use spaces, not tabs",
		},
		{
			name:    "tab after dash",
			input:   "context_files:\n  -\ta.md\nuser_prompt: hi\n",
			wantErr: "line 2: use spaces, not tabs",
		},
		{
			name:  "column zero comment between items",
			input: "context_files:\n  - a.md\n# note\n  - b.md\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a.md", "b.md"}, UserPrompt: "hi"},
		},
		{
			name:    "duplicate path reports second line",
			input:   "context_files:\n  - a.md\n  - b.md\n  - a.md\nuser_prompt: hi\n",
			wantErr: `line 4: duplicate context file "a.md"`,
		},
		{
			name:    "no items",
			input:   "context_files:\nuser_prompt: hi\n",
			wantErr: "line 1: context_files needs at least one item",
		},
		{
			name:  "context_files key with comment only takes items below",
			input: "context_files: # note\n  - a.md\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "hi"},
		},
		{
			name:    "user_prompt key with comment only needs a value",
			input:   "context_files:\n  - a.md\nuser_prompt: # note\n",
			wantErr: "line 3: user_prompt needs a value",
		},
		{
			name:    "user_prompt comment that starts like a list",
			input:   "context_files:\n  - a.md\nuser_prompt: #[x\n",
			wantErr: "line 3: user_prompt needs a value",
		},
		{
			name:    "user_prompt comment after a tab",
			input:   "context_files:\n  - a.md\nuser_prompt: \t# x\n",
			wantErr: "line 3: user_prompt needs a value",
		},
		{
			name:  "context_files comment with no space after the colon",
			input: "context_files:#x\n  - a.md\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "hi"},
		},
		{
			name:    "flow list",
			input:   "context_files: [a, b]\nuser_prompt: hi\n",
			wantErr: "line 1: put each file on its own \"- path\" line, lists like [a, b] aren't supported",
		},
		{
			name:    "nested list item",
			input:   "context_files:\n  - - a.md\nuser_prompt: hi\n",
			wantErr: "line 2: nested lists aren't supported",
		},
		{
			name:    "bare nested list marker",
			input:   "context_files:\n  - -\nuser_prompt: hi\n",
			wantErr: "line 2: nested lists aren't supported",
		},
		{
			name:    "nested list marker followed by a tab",
			input:   "context_files:\n  - -\ta.md\nuser_prompt: hi\n",
			wantErr: "line 2: nested lists aren't supported",
		},
		{
			name:  "block prompt with two lines",
			input: "context_files:\n  - a.md\nuser_prompt: |\n  line one\n  line two\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "line one\nline two\n"},
		},
		{
			name:  "block prompt strips indent and keeps extra spaces",
			input: "context_files:\n  - a.md\nuser_prompt: |\n    first\n      deeper\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "first\n  deeper\n"},
		},
		{
			name:    "block prompt line indented less than the first line",
			input:   "context_files:\n  - a.md\nuser_prompt: |\n    first\n  short\n",
			wantErr: "line 5: block line is indented less than the first line",
		},
		{
			name:  "block prompt keeps leading and inner blank lines",
			input: "context_files:\n  - a.md\nuser_prompt: |\n\n  one\n\n  two\n\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "\none\n\ntwo\n"},
		},
		{
			name:  "block prompt keeps indented hash line as text",
			input: "context_files:\n  - a.md\nuser_prompt: |\n  # Heading\n  body\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "# Heading\nbody\n"},
		},
		{
			name:    "column zero hash ends block prompt",
			input:   "context_files:\n  - a.md\nuser_prompt: |\n  one\n# note\n  two\n",
			wantErr: "line 6: indented line outside a key",
		},
		{
			name:  "block prompt ends at next top-level key",
			input: "user_prompt: |\n  one\ncontext_files:\n  - a.md\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "one\n"},
		},
		{
			name:  "block prompt with trailing comment on indicator line",
			input: "context_files:\n  - a.md\nuser_prompt: | # note\n  hi\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "hi\n"},
		},
		{
			name:    "folded prompt is not supported",
			input:   "context_files:\n  - a.md\nuser_prompt: >\n  one\n",
			wantErr: "line 3: use | or a plain value, > isn't supported",
		},
		{
			name:    "strip indicator is not supported",
			input:   "context_files:\n  - a.md\nuser_prompt: |-\n  one\n",
			wantErr: `line 3: use a plain |, "-" after it isn't supported`,
		},
		{
			name:    "keep indicator is not supported",
			input:   "context_files:\n  - a.md\nuser_prompt: |+\n  one\n",
			wantErr: `line 3: use a plain |, "+" after it isn't supported`,
		},
		{
			name:    "indent indicator is not supported",
			input:   "context_files:\n  - a.md\nuser_prompt: |2\n    one\n",
			wantErr: `line 3: use a plain |, "2" after it isn't supported`,
		},
		{
			name:    "block prompt with no content",
			input:   "context_files:\n  - a.md\nuser_prompt: |\n",
			wantErr: "line 3: block under user_prompt is empty",
		},
		{
			name:    "block prompt with only blank lines",
			input:   "context_files:\n  - a.md\nuser_prompt: |\n\n   \n",
			wantErr: "line 3: block under user_prompt is empty",
		},
		{
			name:  "block prompt keeps extra spaces on a whitespace-only line",
			input: "context_files:\n  - a.md\nuser_prompt: |\n    one\n      \n    two\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "one\n  \ntwo\n"},
		},
		{
			name:    "tab before block line",
			input:   "context_files:\n  - a.md\nuser_prompt: |\n\tone\n",
			wantErr: "line 4: use spaces, not tabs",
		},
		{
			name:    "escape in double quoted prompt",
			input:   "context_files:\n  - a.md\nuser_prompt: \"a\\nb\"\n",
			wantErr: "line 3: escapes in double quotes aren't supported",
		},
		{
			name:    "escape in double quoted path",
			input:   "context_files:\n  - \"a\\b.md\"\nuser_prompt: hi\n",
			wantErr: "line 2: escapes in double quotes aren't supported",
		},
		{
			name:  "backslash in single quoted path is literal",
			input: "context_files:\n  - 'a\\b.md'\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{`a\b.md`}, UserPrompt: "hi"},
		},
		{
			name:    "empty quoted prompt",
			input:   "context_files:\n  - a.md\nuser_prompt: \"\"\n",
			wantErr: "line 3: user_prompt needs a value",
		},
		{
			name:  "double quoted prompt keeps colon",
			input: "context_files:\n  - a.md\nuser_prompt: \"Explain: hooks\"\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "Explain: hooks"},
		},
		{
			name:  "single quoted prompt with trailing comment",
			input: "context_files:\n  - a.md\nuser_prompt: 'Explain hooks' # note\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "Explain hooks"},
		},
		{
			name:  "plain prompt with trailing comment stripped",
			input: "context_files:\n  - a.md\nuser_prompt: Explain hooks # note\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "Explain hooks"},
		},
		{
			name:    "unclosed quote in prompt",
			input:   "context_files:\n  - a.md\nuser_prompt: \"Explain hooks\n",
			wantErr: "line 3: unclosed quote",
		},
		{
			name:    "tab in blank block line after content",
			input:   "context_files:\n  - a.md\nuser_prompt: |\n  one\n\t\n  two\n",
			wantErr: "line 5: use spaces, not tabs",
		},
		{
			name:    "tab in blank block line before content",
			input:   "context_files:\n  - a.md\nuser_prompt: |\n\t\n  one\n",
			wantErr: "line 4: use spaces, not tabs",
		},
		{
			name:    "tab in indent of blank block line",
			input:   "context_files:\n  - a.md\nuser_prompt: |\n    one\n  \t\n    two\n",
			wantErr: "line 5: use spaces, not tabs",
		},
		{
			name:  "tab after the indent of a blank block line is kept",
			input: "context_files:\n  - a.md\nuser_prompt: |\n  one\n  \t\n  two\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "one\n\t\ntwo\n"},
		},
		{
			name:  "byte order mark before context_files",
			input: "\uFEFFcontext_files:\n  - a.md\nuser_prompt: hi\n",
			want:  Metadata{ContextFiles: []string{"a.md"}, UserPrompt: "hi"},
		},
		{
			name:    "byte order mark before unknown key",
			input:   "\uFEFFmodel: x\ncontext_files:\n  - a.md\nuser_prompt: hi\n",
			wantErr: `line 1: unknown key "model"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Parse([]byte(test.input))
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("Parse() error = nil, want error containing %q", test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("Parse() error = %q, want it to contain %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("Parse() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseRejectsPlainValueStartingWithSpecialCharacter(t *testing.T) {
	for _, char := range promptSpecialCharacters {
		value := string(char)
		t.Run("prompt "+value, func(t *testing.T) {
			input := "context_files:\n  - a.md\nuser_prompt: " + value + "x\n"
			want := `line 3: a value can't start with "` + value + `", wrap it in quotes`
			_, err := Parse([]byte(input))
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("Parse() error = %v, want it to contain %q", err, want)
			}
		})
	}
	for _, char := range pathSpecialCharacters {
		value := string(char)
		t.Run("path "+value, func(t *testing.T) {
			input := "context_files:\n  - " + value + "x.md\nuser_prompt: hi\n"
			want := `line 2: a value can't start with "` + value + `", wrap it in quotes`
			_, err := Parse([]byte(input))
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("Parse() error = %v, want it to contain %q", err, want)
			}
		})
	}
}

func TestParseAllowsQuotedValueStartingWithSpecialCharacter(t *testing.T) {
	inputs := []string{
		"context_files:\n  - \"[a].md\"\nuser_prompt: hi\n",
		"context_files:\n  - '[a].md'\nuser_prompt: hi\n",
		"context_files:\n  - a.md\nuser_prompt: \"[x]\"\n",
		"context_files:\n  - a.md\nuser_prompt: '[x]'\n",
		"context_files:\n  - \"- a.md\"\nuser_prompt: hi\n",
	}
	for _, input := range inputs {
		if _, err := Parse([]byte(input)); err != nil {
			t.Errorf("Parse(%q) error = %v, want nil", input, err)
		}
	}
}
