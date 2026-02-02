// internal/context/loader.go
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MaxFileSize is the maximum file size we'll load (1MB)
const MaxFileSize = 1024 * 1024

// LoadFile reads file content with size limits
func LoadFile(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if err := ValidatePath(absPath); err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, use SummarizeDir instead")
	}

	if info.Size() > MaxFileSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), MaxFileSize)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// FormatForContext formats file content for model context with path header
func FormatForContext(path, content string) string {
	var sb strings.Builder

	// Add path header
	sb.WriteString(fmt.Sprintf("=== File: %s ===\n", path))

	// Add line numbers for code files
	if isCodeFile(path) {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			sb.WriteString(fmt.Sprintf("%4d | %s\n", i+1, line))
		}
	} else {
		sb.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			sb.WriteString("\n")
		}
	}

	sb.WriteString(fmt.Sprintf("=== End: %s ===\n", filepath.Base(path)))

	return sb.String()
}

// SummarizeDir returns a tree structure of a directory
func SummarizeDir(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if err := ValidatePath(absPath); err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Directory: %s ===\n", absPath))

	err = walkDir(absPath, "", &sb, 0, 3) // max depth 3
	if err != nil {
		return "", err
	}

	return sb.String(), nil
}

// walkDir recursively walks directory and builds tree representation
func walkDir(path, prefix string, sb *strings.Builder, depth, maxDepth int) error {
	if depth > maxDepth {
		sb.WriteString(prefix + "  ...\n")
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	// Sort entries: directories first, then files
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	// Filter out hidden files and common excludes
	var filtered []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if isExcludedDir(name) {
			continue
		}
		filtered = append(filtered, e)
	}

	for i, entry := range filtered {
		isLast := i == len(filtered)-1
		connector := "|-"
		if isLast {
			connector = "`-"
		}

		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}

		sb.WriteString(fmt.Sprintf("%s%s %s\n", prefix, connector, name))

		if entry.IsDir() {
			newPrefix := prefix + "  "
			if !isLast {
				newPrefix = prefix + "| "
			}
			subPath := filepath.Join(path, entry.Name())
			if err := walkDir(subPath, newPrefix, sb, depth+1, maxDepth); err != nil {
				// Continue on error (permission denied, etc)
				sb.WriteString(newPrefix + "  (error reading)\n")
			}
		}
	}

	return nil
}

// ValidatePath checks for security issues with the path
func ValidatePath(path string) error {
	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check for path traversal attempts
	cleanPath := filepath.Clean(absPath)
	if cleanPath != absPath {
		// Path had traversal components that got cleaned
		if strings.Contains(path, "..") {
			return fmt.Errorf("path traversal not allowed")
		}
	}

	// Check path exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", absPath)
	} else if err != nil {
		return fmt.Errorf("cannot access path: %w", err)
	}

	// Block sensitive paths
	if isSensitivePath(absPath) {
		return fmt.Errorf("access to sensitive path denied")
	}

	return nil
}

// LoadContext loads a file or directory summary and formats it for context
// This is the main integration point for /context add
func LoadContext(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if err := ValidatePath(absPath); err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		return SummarizeDir(absPath)
	}

	content, err := LoadFile(absPath)
	if err != nil {
		return "", err
	}

	return FormatForContext(absPath, content), nil
}

// isCodeFile returns true if the file appears to be source code
func isCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	codeExts := map[string]bool{
		".go":    true,
		".py":    true,
		".js":    true,
		".ts":    true,
		".jsx":   true,
		".tsx":   true,
		".rs":    true,
		".c":     true,
		".h":     true,
		".cpp":   true,
		".hpp":   true,
		".java":  true,
		".rb":    true,
		".php":   true,
		".sh":    true,
		".bash":  true,
		".zsh":   true,
		".yaml":  true,
		".yml":   true,
		".json":  true,
		".toml":  true,
		".sql":   true,
		".lua":   true,
		".vim":   true,
		".el":    true,
		".lisp":  true,
		".zig":   true,
		".nim":   true,
		".swift": true,
		".kt":    true,
		".scala": true,
		".ml":    true,
		".hs":    true,
	}
	return codeExts[ext]
}

// isExcludedDir returns true for directories that should be skipped
func isExcludedDir(name string) bool {
	excluded := map[string]bool{
		"node_modules": true,
		"vendor":       true,
		"__pycache__":  true,
		".git":         true,
		".svn":         true,
		".hg":          true,
		"target":       true,
		"build":        true,
		"dist":         true,
		"bin":          true,
		"obj":          true,
		".idea":        true,
		".vscode":      true,
		".cache":       true,
		"venv":         true,
		".venv":        true,
		"env":          true,
		".env":         true,
	}
	return excluded[name]
}

// isSensitivePath returns true for paths that should never be loaded
func isSensitivePath(path string) bool {
	sensitive := []string{
		"/.ssh/",
		"/.gnupg/",
		"/.aws/",
		"/.config/gcloud",
		"/etc/shadow",
		"/etc/passwd",
		"/.netrc",
		"/.npmrc",
		"/.pypirc",
		"/credentials",
		"/secrets",
		"/.env",
		".pem",
		".key",
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
		"id_dsa",
	}

	lowerPath := strings.ToLower(path)
	for _, s := range sensitive {
		if strings.Contains(lowerPath, strings.ToLower(s)) {
			return true
		}
	}

	return false
}
