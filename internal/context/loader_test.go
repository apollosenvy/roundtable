// internal/context/loader_test.go
package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePath(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "context-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid file",
			path:    testFile,
			wantErr: false,
		},
		{
			name:    "valid directory",
			path:    tmpDir,
			wantErr: false,
		},
		{
			name:    "nonexistent path",
			path:    filepath.Join(tmpDir, "nonexistent"),
			wantErr: true,
		},
		{
			name:    "path traversal",
			path:    filepath.Join(tmpDir, "..", "..", "etc", "passwd"),
			wantErr: true, // either traversal blocked or doesn't exist
		},
		{
			name:    "sensitive ssh path",
			path:    "/home/user/.ssh/id_rsa",
			wantErr: true,
		},
		{
			name:    "sensitive env file",
			path:    "/path/to/.env",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestLoadFile(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "context-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	testContent := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	t.Run("load valid file", func(t *testing.T) {
		content, err := LoadFile(testFile)
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}
		if content != testContent {
			t.Errorf("LoadFile() content mismatch\ngot:  %q\nwant: %q", content, testContent)
		}
	})

	t.Run("load directory fails", func(t *testing.T) {
		_, err := LoadFile(tmpDir)
		if err == nil {
			t.Error("LoadFile() expected error for directory")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Errorf("LoadFile() error should mention directory, got: %v", err)
		}
	})

	t.Run("load nonexistent fails", func(t *testing.T) {
		_, err := LoadFile(filepath.Join(tmpDir, "nonexistent.go"))
		if err == nil {
			t.Error("LoadFile() expected error for nonexistent file")
		}
	})
}

func TestFormatForContext(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		content      string
		wantContains []string
	}{
		{
			name:    "code file with line numbers",
			path:    "/project/main.go",
			content: "package main\n\nfunc main() {}\n",
			wantContains: []string{
				"=== File: /project/main.go ===",
				"   1 | package main",
				"   3 | func main() {}",
				"=== End: main.go ===",
			},
		},
		{
			name:    "text file without line numbers",
			path:    "/project/README.md",
			content: "# Title\n\nSome content\n",
			wantContains: []string{
				"=== File: /project/README.md ===",
				"# Title",
				"Some content",
				"=== End: README.md ===",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatForContext(tt.path, tt.content)
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("FormatForContext() missing expected content: %q\ngot: %s", want, result)
				}
			}
		})
	}
}

func TestSummarizeDir(t *testing.T) {
	// Create temp directory structure for testing
	tmpDir, err := os.MkdirTemp("", "context-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create directory structure
	dirs := []string{
		"src",
		"src/internal",
		"docs",
		"node_modules", // should be excluded
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Create files
	files := map[string]string{
		"main.go":           "package main",
		"src/lib.go":        "package src",
		"src/internal/a.go": "package internal",
		"docs/README.md":    "# Docs",
		".hidden":           "hidden file", // should be excluded
	}
	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", name, err)
		}
	}

	t.Run("summarize directory", func(t *testing.T) {
		result, err := SummarizeDir(tmpDir)
		if err != nil {
			t.Fatalf("SummarizeDir() error = %v", err)
		}

		// Should contain these
		want := []string{
			"=== Directory:",
			"src/",
			"docs/",
			"main.go",
			"lib.go",
			"README.md",
		}
		for _, w := range want {
			if !strings.Contains(result, w) {
				t.Errorf("SummarizeDir() missing expected: %q\ngot: %s", w, result)
			}
		}

		// Should NOT contain these (excluded)
		notWant := []string{
			"node_modules",
			".hidden",
		}
		for _, nw := range notWant {
			if strings.Contains(result, nw) {
				t.Errorf("SummarizeDir() should not contain: %q\ngot: %s", nw, result)
			}
		}
	})

	t.Run("summarize file fails", func(t *testing.T) {
		_, err := SummarizeDir(filepath.Join(tmpDir, "main.go"))
		if err == nil {
			t.Error("SummarizeDir() expected error for file")
		}
	})
}

func TestLoadContext(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "context-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package test\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	t.Run("load file context", func(t *testing.T) {
		result, err := LoadContext(testFile)
		if err != nil {
			t.Fatalf("LoadContext() error = %v", err)
		}
		if !strings.Contains(result, "=== File:") {
			t.Error("LoadContext() for file should contain file header")
		}
		if !strings.Contains(result, "package test") {
			t.Error("LoadContext() should contain file content")
		}
	})

	t.Run("load dir context", func(t *testing.T) {
		result, err := LoadContext(tmpDir)
		if err != nil {
			t.Fatalf("LoadContext() error = %v", err)
		}
		if !strings.Contains(result, "=== Directory:") {
			t.Error("LoadContext() for dir should contain directory header")
		}
	})
}

func TestIsCodeFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"script.py", true},
		{"app.js", true},
		{"lib.rs", true},
		{"README.md", false},
		{"image.png", false},
		{"data.csv", false},
		{"config.yaml", true},
		{"settings.json", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isCodeFile(tt.path); got != tt.want {
				t.Errorf("isCodeFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/home/user/.ssh/id_rsa", true},
		{"/home/user/.ssh/config", true},
		{"/home/user/.gnupg/private.key", true},
		{"/home/user/.aws/credentials", true},
		{"/path/to/.env", true},
		{"/project/secrets/api.key", true},
		{"/project/main.go", false},
		{"/project/config.yaml", false},
		{"/home/user/projects/test.py", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isSensitivePath(tt.path); got != tt.want {
				t.Errorf("isSensitivePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
