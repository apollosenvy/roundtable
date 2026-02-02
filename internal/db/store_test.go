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
}
