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

func TestIsPathUnder_SymlinkBypass(t *testing.T) {
	tmpDir := t.TempDir()
	allowed := filepath.Join(tmpDir, "allowed")
	os.MkdirAll(allowed, 0755)

	// Create a target outside the allowed directory.
	outside := filepath.Join(tmpDir, "outside")
	os.MkdirAll(outside, 0755)
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0600)

	// Create a symlink inside allowed that points outside.
	link := filepath.Join(allowed, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// /allowed/link/secret.txt should NOT be considered under /allowed
	// because the symlink resolves to /outside/secret.txt.
	if isPathUnder(filepath.Join(link, "secret.txt"), []string{allowed}) {
		t.Error("symlink bypass: path through symlink should not be considered under allowed dir")
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
