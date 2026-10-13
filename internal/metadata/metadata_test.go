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
		UserPrompt:   "Explain how Claude Code hooks work.",
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
