package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolError(t *testing.T) {
	result := toolError("something went wrong")
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
}

func TestIsPathUnder(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	os.MkdirAll(subDir, 0755)

	tests := []struct {
		name    string
		path    string
		allowed []string
		want    bool
	}{
		{"exact match", tmpDir, []string{tmpDir}, true},
		{"subdirectory", filepath.Join(tmpDir, "sub", "file.txt"), []string{tmpDir}, true},
		{"outside", "/etc/passwd", []string{tmpDir}, false},
		{"traversal attempt", filepath.Join(tmpDir, "..", "etc", "passwd"), []string{tmpDir}, false},
		{"empty allowed", tmpDir, nil, false},
		{"multiple allowed", filepath.Join(subDir, "file.txt"), []string{"/nonexistent", subDir}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPathUnder(tt.path, tt.allowed)
			if got != tt.want {
				t.Errorf("isPathUnder(%q, %v) = %v, want %v", tt.path, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestPtrBool(t *testing.T) {
	v := ptrBool(true)
	if v == nil || *v != true {
		t.Error("ptrBool(true) should return pointer to true")
	}
	v = ptrBool(false)
	if v == nil || *v != false {
		t.Error("ptrBool(false) should return pointer to false")
	}
}
