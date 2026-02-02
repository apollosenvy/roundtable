// internal/db/store_test.go
package db

import (
	"os"
	"testing"
)

func TestStore(t *testing.T) {
	// Use temp dir for test
	os.Setenv("XDG_DATA_HOME", t.TempDir())

	store, err := Open()
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer store.Close()

	// Test create debate
	err = store.CreateDebate("test-1", "Test Debate", "/home/test/project")
	if err != nil {
		t.Fatalf("CreateDebate() failed: %v", err)
	}

	// Test get debate
	debate, err := store.GetDebate("test-1")
	if err != nil {
		t.Fatalf("GetDebate() failed: %v", err)
	}
	if debate.Name != "Test Debate" {
		t.Errorf("Expected name 'Test Debate', got %s", debate.Name)
	}

	// Test add message
	msgID, err := store.AddMessage("test-1", "claude", "Hello from Claude", "model")
	if err != nil {
		t.Fatalf("AddMessage() failed: %v", err)
	}
	if msgID == 0 {
		t.Error("Expected non-zero message ID")
	}

	// Test get messages
	messages, err := store.GetMessages("test-1")
	if err != nil {
		t.Fatalf("GetMessages() failed: %v", err)
	}
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	// Test list debates
	debates, err := store.ListDebates()
	if err != nil {
		t.Fatalf("ListDebates() failed: %v", err)
	}
	if len(debates) != 1 {
		t.Errorf("Expected 1 debate, got %d", len(debates))
	}

	// Test add context file
	err = store.AddContextFile("test-1", "/path/to/file.go", "package main\n\nfunc main() {}")
	if err != nil {
		t.Fatalf("AddContextFile() failed: %v", err)
	}

	// Test get context files
	contextFiles, err := store.GetContextFiles("test-1")
	if err != nil {
		t.Fatalf("GetContextFiles() failed: %v", err)
	}
	if len(contextFiles) != 1 {
		t.Errorf("Expected 1 context file, got %d", len(contextFiles))
	}
	if contextFiles[0].Path != "/path/to/file.go" {
		t.Errorf("Expected path '/path/to/file.go', got %s", contextFiles[0].Path)
	}

	// Test update debate status
	err = store.UpdateDebateStatus("test-1", "resolved", "Consensus reached on approach A")
	if err != nil {
		t.Fatalf("UpdateDebateStatus() failed: %v", err)
	}
	debate, err = store.GetDebate("test-1")
	if err != nil {
		t.Fatalf("GetDebate() after status update failed: %v", err)
	}
	if debate.Status != "resolved" {
		t.Errorf("Expected status 'resolved', got %s", debate.Status)
	}
	if debate.Consensus != "Consensus reached on approach A" {
		t.Errorf("Expected consensus text, got %s", debate.Consensus)
	}

	// Test update debate name
	err = store.UpdateDebateName("test-1", "Renamed Debate")
	if err != nil {
		t.Fatalf("UpdateDebateName() failed: %v", err)
	}
	debate, err = store.GetDebate("test-1")
	if err != nil {
		t.Fatalf("GetDebate() after name update failed: %v", err)
	}
	if debate.Name != "Renamed Debate" {
		t.Errorf("Expected name 'Renamed Debate', got %s", debate.Name)
	}

	// Test remove context file
	err = store.RemoveContextFile("test-1", "/path/to/file.go")
	if err != nil {
		t.Fatalf("RemoveContextFile() failed: %v", err)
	}
	contextFiles, err = store.GetContextFiles("test-1")
	if err != nil {
		t.Fatalf("GetContextFiles() after remove failed: %v", err)
	}
	if len(contextFiles) != 0 {
		t.Errorf("Expected 0 context files after removal, got %d", len(contextFiles))
	}
}
