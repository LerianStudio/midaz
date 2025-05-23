package audit

import (
	"errors"
	"testing"
	"time"
)

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder()

	if builder == nil {
		t.Fatal("Expected builder to not be nil")
	}

	// Check initial state
	entry := builder.Build()

	if entry.Flags == nil {
		t.Error("Expected Flags map to be initialized")
	}

	if entry.Metadata == nil {
		t.Error("Expected Metadata map to be initialized")
	}

	if len(entry.Flags) != 0 {
		t.Errorf("Expected empty Flags map, got %d items", len(entry.Flags))
	}

	if len(entry.Metadata) != 0 {
		t.Errorf("Expected empty Metadata map, got %d items", len(entry.Metadata))
	}
}

func TestBuilder_WithCommand(t *testing.T) {
	builder := NewBuilder()

	result := builder.WithCommand("test-command")

	// Should return self for chaining
	if result != builder {
		t.Error("WithCommand should return self for chaining")
	}

	entry := builder.Build()
	if entry.Command != "test-command" {
		t.Errorf("Expected command 'test-command', got '%s'", entry.Command)
	}
}

func TestBuilder_WithArgs(t *testing.T) {
	builder := NewBuilder()
	args := []string{"arg1", "arg2", "arg3"}

	result := builder.WithArgs(args)

	// Should return self for chaining
	if result != builder {
		t.Error("WithArgs should return self for chaining")
	}

	entry := builder.Build()
	if len(entry.Args) != 3 {
		t.Errorf("Expected 3 args, got %d", len(entry.Args))
	}

	for i, arg := range args {
		if entry.Args[i] != arg {
			t.Errorf("Expected arg[%d] '%s', got '%s'", i, arg, entry.Args[i])
		}
	}
}

func TestBuilder_WithFlag(t *testing.T) {
	builder := NewBuilder()

	result := builder.WithFlag("key1", "value1")

	// Should return self for chaining
	if result != builder {
		t.Error("WithFlag should return self for chaining")
	}

	entry := builder.Build()
	if entry.Flags["key1"] != "value1" {
		t.Errorf("Expected flag 'key1' = 'value1', got '%s'", entry.Flags["key1"])
	}

	// Test multiple flags
	builder.WithFlag("key2", "value2")
	entry = builder.Build()

	if len(entry.Flags) != 2 {
		t.Errorf("Expected 2 flags, got %d", len(entry.Flags))
	}

	if entry.Flags["key2"] != "value2" {
		t.Errorf("Expected flag 'key2' = 'value2', got '%s'", entry.Flags["key2"])
	}
}

func TestBuilder_WithFlags(t *testing.T) {
	builder := NewBuilder()
	flags := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	result := builder.WithFlags(flags)

	// Should return self for chaining
	if result != builder {
		t.Error("WithFlags should return self for chaining")
	}

	entry := builder.Build()
	if len(entry.Flags) != 3 {
		t.Errorf("Expected 3 flags, got %d", len(entry.Flags))
	}

	for key, expectedValue := range flags {
		if actualValue, exists := entry.Flags[key]; !exists {
			t.Errorf("Expected flag '%s' to exist", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected flag '%s' = '%s', got '%s'", key, expectedValue, actualValue)
		}
	}
}

func TestBuilder_WithFlags_OverwriteExisting(t *testing.T) {
	builder := NewBuilder()

	// Set initial flag
	builder.WithFlag("key1", "original")

	// Overwrite with WithFlags
	flags := map[string]string{
		"key1": "overwritten",
		"key2": "new",
	}
	builder.WithFlags(flags)

	entry := builder.Build()

	if entry.Flags["key1"] != "overwritten" {
		t.Errorf("Expected flag 'key1' to be overwritten, got '%s'", entry.Flags["key1"])
	}

	if entry.Flags["key2"] != "new" {
		t.Errorf("Expected flag 'key2' = 'new', got '%s'", entry.Flags["key2"])
	}
}

func TestBuilder_WithResult(t *testing.T) {
	builder := NewBuilder()

	result := builder.WithResult("success")

	// Should return self for chaining
	if result != builder {
		t.Error("WithResult should return self for chaining")
	}

	entry := builder.Build()
	if entry.Result != "success" {
		t.Errorf("Expected result 'success', got '%s'", entry.Result)
	}
}

func TestBuilder_WithError(t *testing.T) {
	builder := NewBuilder()
	testErr := errors.New("test error message")

	result := builder.WithError(testErr)

	// Should return self for chaining
	if result != builder {
		t.Error("WithError should return self for chaining")
	}

	entry := builder.Build()
	if entry.Error != "test error message" {
		t.Errorf("Expected error 'test error message', got '%s'", entry.Error)
	}

	if entry.Result != "error" {
		t.Errorf("Expected result 'error', got '%s'", entry.Result)
	}
}

func TestBuilder_WithError_Nil(t *testing.T) {
	builder := NewBuilder()

	// Set initial result
	builder.WithResult("success")

	result := builder.WithError(nil)

	// Should return self for chaining
	if result != builder {
		t.Error("WithError should return self for chaining")
	}

	entry := builder.Build()

	// Should not change anything with nil error
	if entry.Error != "" {
		t.Errorf("Expected empty error with nil, got '%s'", entry.Error)
	}

	if entry.Result != "success" {
		t.Errorf("Expected result to remain 'success', got '%s'", entry.Result)
	}
}

func TestBuilder_WithDuration(t *testing.T) {
	builder := NewBuilder()
	duration := 5 * time.Second

	result := builder.WithDuration(duration)

	// Should return self for chaining
	if result != builder {
		t.Error("WithDuration should return self for chaining")
	}

	entry := builder.Build()
	if entry.Duration != duration {
		t.Errorf("Expected duration %v, got %v", duration, entry.Duration)
	}
}

func TestBuilder_WithMetadata(t *testing.T) {
	builder := NewBuilder()

	result := builder.WithMetadata("key1", "value1")

	// Should return self for chaining
	if result != builder {
		t.Error("WithMetadata should return self for chaining")
	}

	entry := builder.Build()
	if entry.Metadata["key1"] != "value1" {
		t.Errorf("Expected metadata 'key1' = 'value1', got '%s'", entry.Metadata["key1"])
	}

	// Test multiple metadata
	builder.WithMetadata("key2", "value2")
	entry = builder.Build()

	if len(entry.Metadata) != 2 {
		t.Errorf("Expected 2 metadata items, got %d", len(entry.Metadata))
	}

	if entry.Metadata["key2"] != "value2" {
		t.Errorf("Expected metadata 'key2' = 'value2', got '%s'", entry.Metadata["key2"])
	}
}

func TestBuilder_WithUndo(t *testing.T) {
	builder := NewBuilder()
	undoCommand := "test undo command"

	result := builder.WithUndo(undoCommand)

	// Should return self for chaining
	if result != builder {
		t.Error("WithUndo should return self for chaining")
	}

	entry := builder.Build()
	if !entry.Undoable {
		t.Error("Expected Undoable to be true")
	}

	if entry.UndoCommand != undoCommand {
		t.Errorf("Expected UndoCommand '%s', got '%s'", undoCommand, entry.UndoCommand)
	}
}

func TestBuilder_ChainedMethods(t *testing.T) {
	// Test method chaining
	builder := NewBuilder()

	entry := builder.
		WithCommand("test").
		WithArgs([]string{"arg1", "arg2"}).
		WithFlag("flag1", "value1").
		WithFlags(map[string]string{"flag2": "value2"}).
		WithResult("success").
		WithDuration(time.Second).
		WithMetadata("meta1", "metavalue1").
		WithUndo("undo command").
		Build()

	// Verify all fields are set correctly
	if entry.Command != "test" {
		t.Errorf("Expected command 'test', got '%s'", entry.Command)
	}

	if len(entry.Args) != 2 || entry.Args[0] != "arg1" || entry.Args[1] != "arg2" {
		t.Errorf("Expected args ['arg1', 'arg2'], got %v", entry.Args)
	}

	if entry.Flags["flag1"] != "value1" {
		t.Errorf("Expected flag1 'value1', got '%s'", entry.Flags["flag1"])
	}

	if entry.Flags["flag2"] != "value2" {
		t.Errorf("Expected flag2 'value2', got '%s'", entry.Flags["flag2"])
	}

	if entry.Result != "success" {
		t.Errorf("Expected result 'success', got '%s'", entry.Result)
	}

	if entry.Duration != time.Second {
		t.Errorf("Expected duration 1s, got %v", entry.Duration)
	}

	if entry.Metadata["meta1"] != "metavalue1" {
		t.Errorf("Expected metadata 'metavalue1', got '%s'", entry.Metadata["meta1"])
	}

	if !entry.Undoable {
		t.Error("Expected Undoable to be true")
	}

	if entry.UndoCommand != "undo command" {
		t.Errorf("Expected UndoCommand 'undo command', got '%s'", entry.UndoCommand)
	}
}

func TestBuilder_Build_MultipleBuilds(t *testing.T) {
	builder := NewBuilder()
	builder.WithCommand("test")

	// Build multiple times
	entry1 := builder.Build()
	entry2 := builder.Build()

	// Both should have the same values
	if entry1.Command != entry2.Command {
		t.Error("Multiple builds should return the same entry")
	}

	// Modify builder after first build
	builder.WithResult("success")
	entry3 := builder.Build()

	// Third build should reflect the change
	if entry3.Result != "success" {
		t.Errorf("Expected result 'success' after modification, got '%s'", entry3.Result)
	}
}
