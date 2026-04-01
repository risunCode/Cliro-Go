package rules

import (
	"reflect"
	"testing"
)

func TestRemapToolCallArgs(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]any
		expected map[string]any
	}{
		{
			name:     "grep query and paths",
			toolName: "Grep",
			args:     map[string]any{"query": "needle", "paths": []any{"src"}},
			expected: map[string]any{"pattern": "needle", "path": "src"},
		},
		{
			name:     "glob description remap",
			toolName: "glob",
			args:     map[string]any{"description": "*.go", "paths": []any{"internal"}},
			expected: map[string]any{"pattern": "*.go", "path": "internal"},
		},
		{
			name:     "read path remap",
			toolName: "Read",
			args:     map[string]any{"path": "main.go"},
			expected: map[string]any{"file_path": "main.go"},
		},
		{
			name:     "ls default path",
			toolName: "LS",
			args:     map[string]any{},
			expected: map[string]any{"path": "."},
		},
		{
			name:     "enter plan mode strips args",
			toolName: "EnterPlanMode",
			args:     map[string]any{"reason": "plan"},
			expected: map[string]any{},
		},
		{
			name:     "generic single path fallback",
			toolName: "Bash",
			args:     map[string]any{"paths": []any{"workspace"}},
			expected: map[string]any{"paths": []any{"workspace"}, "path": "workspace"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RemapToolCallArgs(tc.toolName, tc.args)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Fatalf("RemapToolCallArgs(%q) = %#v, want %#v", tc.toolName, got, tc.expected)
			}
		})
	}
}
