package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig should not return nil")
	}

	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}

	if config.MaxEntries != 10000 {
		t.Errorf("Expected MaxEntries 10000, got %d", config.MaxEntries)
	}

	// Should contain .mdz directory
	if !filepath.IsAbs(config.FilePath) {
		t.Error("Expected absolute file path")
	}
}

func TestNew_WithConfig(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_audit.json"),
		MaxEntries: 100,
	}

	trail, err := New(config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if trail == nil {
		t.Fatal("Expected trail to not be nil")
	}

	if trail.maxEntries != 100 {
		t.Errorf("Expected maxEntries 100, got %d", trail.maxEntries)
	}

	if !trail.initialized {
		t.Error("Expected trail to be initialized")
	}
}

func TestNew_WithNilConfig(t *testing.T) {
	trail, err := New(nil)

	if err != nil {
		t.Errorf("Expected no error with nil config, got %v", err)
	}

	if trail == nil {
		t.Fatal("Expected trail to not be nil")
	}
}

func TestEntry_Fields(t *testing.T) {
	entry := Entry{
		ID:          "test-123",
		Timestamp:   time.Now(),
		Command:     "test",
		Args:        []string{"arg1", "arg2"},
		Flags:       map[string]string{"flag1": "value1"},
		User:        "testuser",
		Result:      "success",
		Error:       "",
		Duration:    time.Second,
		Metadata:    map[string]string{"meta1": "value1"},
		Undoable:    true,
		UndoCommand: "test undo",
	}

	if entry.ID != "test-123" {
		t.Errorf("Expected ID 'test-123', got %s", entry.ID)
	}

	if entry.Command != "test" {
		t.Errorf("Expected Command 'test', got %s", entry.Command)
	}

	if len(entry.Args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(entry.Args))
	}

	if !entry.Undoable {
		t.Error("Expected Undoable to be true")
	}
}

func TestTrail_LogCommand(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_audit.json"),
		MaxEntries: 100,
	}

	trail, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	entry := Entry{
		Command: "test",
		Args:    []string{"arg1"},
		Result:  "success",
	}

	err = trail.LogCommand(entry)
	if err != nil {
		t.Errorf("Expected no error logging command, got %v", err)
	}

	// Check that entry was added
	history := trail.GetHistory(10)
	if len(history) != 1 {
		t.Errorf("Expected 1 entry in history, got %d", len(history))
	}

	// Verify generated ID and timestamp
	if history[0].ID == "" {
		t.Error("Expected ID to be generated")
	}

	if history[0].Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestTrail_LogCommand_MaxEntries(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_audit.json"),
		MaxEntries: 2, // Small limit for testing
	}

	trail, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	// Add 3 entries (exceeds limit)
	for i := 0; i < 3; i++ {
		entry := Entry{
			Command: fmt.Sprintf("test%d", i),
			Result:  "success",
		}

		err = trail.LogCommand(entry)
		if err != nil {
			t.Errorf("Failed to log command %d: %v", i, err)
		}
	}

	// Should only have 2 entries (max)
	history := trail.GetHistory(10)
	if len(history) != 2 {
		t.Errorf("Expected 2 entries (max), got %d", len(history))
	}

	// Should have the most recent entries
	if history[0].Command != "test2" {
		t.Errorf("Expected most recent command 'test2', got %s", history[0].Command)
	}
}

func TestTrail_GetHistory(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_audit.json"),
		MaxEntries: 100,
	}

	trail, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	// Add multiple entries
	for i := 0; i < 5; i++ {
		entry := Entry{
			Command: fmt.Sprintf("test%d", i),
			Result:  "success",
		}

		trail.LogCommand(entry)
	}

	// Test limit
	history := trail.GetHistory(3)
	if len(history) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(history))
	}

	// Should be in reverse order (most recent first)
	if history[0].Command != "test4" {
		t.Errorf("Expected first entry 'test4', got %s", history[0].Command)
	}

	// Test no limit
	history = trail.GetHistory(0)
	if len(history) != 5 {
		t.Errorf("Expected 5 entries, got %d", len(history))
	}
}

func TestTrail_GetEntry(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_audit.json"),
		MaxEntries: 100,
	}

	trail, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	entry := Entry{
		ID:      "test-123",
		Command: "test",
		Result:  "success",
	}

	trail.LogCommand(entry)

	// Test existing entry
	found, err := trail.GetEntry("test-123")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if found == nil {
		t.Fatal("Expected entry to be found")
	}

	if found.Command != "test" {
		t.Errorf("Expected command 'test', got %s", found.Command)
	}

	// Test non-existing entry
	_, err = trail.GetEntry("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent entry")
	}
}

func TestTrail_GetUndoableCommands(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_audit.json"),
		MaxEntries: 100,
	}

	trail, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	// Add undoable entry
	undoableEntry := Entry{
		Command:     "create",
		Result:      "success",
		Undoable:    true,
		UndoCommand: "delete",
	}
	trail.LogCommand(undoableEntry)

	// Add non-undoable entry
	nonUndoableEntry := Entry{
		Command:  "list",
		Result:   "success",
		Undoable: false,
	}
	trail.LogCommand(nonUndoableEntry)

	// Add failed undoable entry
	failedEntry := Entry{
		Command:     "create",
		Result:      "error",
		Undoable:    true,
		UndoCommand: "delete",
	}
	trail.LogCommand(failedEntry)

	undoable := trail.GetUndoableCommands(10)

	// Should only return successful undoable commands
	if len(undoable) != 1 {
		t.Errorf("Expected 1 undoable command, got %d", len(undoable))
	}

	if undoable[0].Command != "create" {
		t.Errorf("Expected command 'create', got %s", undoable[0].Command)
	}
}

func TestTrail_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_audit.json"),
		MaxEntries: 100,
	}

	trail, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	// Add entries
	for i := 0; i < 3; i++ {
		entry := Entry{
			Command: fmt.Sprintf("test%d", i),
			Result:  "success",
		}
		trail.LogCommand(entry)
	}

	// Verify entries exist
	history := trail.GetHistory(10)
	if len(history) != 3 {
		t.Errorf("Expected 3 entries before clear, got %d", len(history))
	}

	// Clear
	err = trail.Clear()
	if err != nil {
		t.Errorf("Expected no error clearing, got %v", err)
	}

	// Verify entries are cleared
	history = trail.GetHistory(10)
	if len(history) != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", len(history))
	}
}

func TestTrail_LoadSave(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_audit.json")

	// Create initial trail and add entry
	config := &Config{
		Enabled:    true,
		FilePath:   filePath,
		MaxEntries: 100,
	}

	trail1, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	entry := Entry{
		ID:      "test-123",
		Command: "test",
		Result:  "success",
	}

	trail1.LogCommand(entry)

	// Create new trail with same file
	trail2, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create second trail: %v", err)
	}

	// Should load existing entries
	history := trail2.GetHistory(10)
	if len(history) != 1 {
		t.Errorf("Expected 1 loaded entry, got %d", len(history))
	}

	if history[0].ID != "test-123" {
		t.Errorf("Expected loaded entry ID 'test-123', got %s", history[0].ID)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()

	// Sleep to ensure different timestamp
	time.Sleep(1 * time.Nanosecond)

	id2 := generateID()

	if id1 == "" {
		t.Error("Generated ID should not be empty")
	}

	if id2 == "" {
		t.Error("Generated ID should not be empty")
	}

	// IDs should be different (they include nanoseconds)
	if id1 == id2 {
		t.Error("Generated IDs should be unique")
	}
}

func TestTrail_LoadNonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "non_existent.json"),
		MaxEntries: 100,
	}

	// Should not fail when file doesn't exist
	trail, err := New(config)
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got %v", err)
	}

	if trail == nil {
		t.Fatal("Expected trail to be created")
	}

	// Should have empty history
	history := trail.GetHistory(10)
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d entries", len(history))
	}
}

func TestTrail_LoadInvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invalid.json")

	// Create invalid JSON file
	err := os.WriteFile(filePath, []byte("invalid json"), 0644)
	if err != nil {
		t.Fatalf("Failed to create invalid file: %v", err)
	}

	config := &Config{
		Enabled:    true,
		FilePath:   filePath,
		MaxEntries: 100,
	}

	// Should fail to load invalid JSON
	_, err = New(config)
	if err == nil {
		t.Error("Expected error for invalid JSON file")
	}
}

func TestConfig_Fields(t *testing.T) {
	config := &Config{
		Enabled:    false,
		FilePath:   "/test/path",
		MaxEntries: 500,
	}

	if config.Enabled {
		t.Error("Expected Enabled to be false")
	}

	if config.FilePath != "/test/path" {
		t.Errorf("Expected FilePath '/test/path', got %s", config.FilePath)
	}

	if config.MaxEntries != 500 {
		t.Errorf("Expected MaxEntries 500, got %d", config.MaxEntries)
	}
}
