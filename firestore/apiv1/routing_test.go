package firestore

import (
	"testing"
)

func TestBuildRoutingHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Full path",
			input:    "projects/my-project/databases/my-db/documents/my-doc",
			expected: "project_id=my-project&database_id=my-db",
		},
		{
			name:     "Database root",
			input:    "projects/my-project/databases/my-db",
			expected: "project_id=my-project&database_id=my-db",
		},
		{
			name:     "Only project",
			input:    "projects/my-project",
			expected: "project_id=my-project",
		},
		{
			name:     "No match",
			input:    "invalid/path",
			expected: "",
		},
		{
			name:     "Special chars",
			input:    "projects/my%2Fproject/databases/my%2Fdb",
			expected: "project_id=my%252Fproject&database_id=my%252Fdb",
		},
		{
			name:     "Operation name",
			input:    "projects/my-project/databases/my-db/operations/op1",
			expected: "project_id=my-project&database_id=my-db",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildRoutingHeader(tc.input)
			if got != tc.expected {
				t.Errorf("buildRoutingHeader(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
