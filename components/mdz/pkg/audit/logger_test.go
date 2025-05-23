package audit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	logger, err := NewLogger()

	if err != nil {
		t.Errorf("Expected no error creating logger, got %v", err)
	}

	if logger == nil {
		t.Fatal("Expected logger to not be nil")
	}

	if logger.trail == nil {
		t.Error("Expected trail to be initialized")
	}

	// Check skip commands
	if !logger.skipCommands["history"] {
		t.Error("Expected 'history' to be in skip commands")
	}

	if !logger.skipCommands["undo"] {
		t.Error("Expected 'undo' to be in skip commands")
	}

	if !logger.skipCommands["interactive"] {
		t.Error("Expected 'interactive' to be in skip commands")
	}
}

func TestLogger_LogCommand_Success(t *testing.T) {
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	args := []string{"account", "create", "--name", "test"}
	duration := time.Second

	err = logger.LogCommand(args, nil, duration)

	if err != nil {
		t.Errorf("Expected no error logging command, got %v", err)
	}

	// Check that entry was logged
	history := logger.trail.GetHistory(1)
	if len(history) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(history))
	}

	entry := history[0]
	if entry.Command != "account" {
		t.Errorf("Expected command 'account', got %s", entry.Command)
	}

	if entry.Result != "success" {
		t.Errorf("Expected result 'success', got %s", entry.Result)
	}

	if entry.Duration != duration {
		t.Errorf("Expected duration %v, got %v", duration, entry.Duration)
	}
}

func TestLogger_LogCommand_WithError(t *testing.T) {
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	args := []string{"account", "create"}
	testErr := errors.New("test error")

	err = logger.LogCommand(args, testErr, time.Second)

	if err != nil {
		t.Errorf("Expected no error logging command, got %v", err)
	}

	// Check that entry was logged with error
	history := logger.trail.GetHistory(1)
	if len(history) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(history))
	}

	entry := history[0]
	if entry.Result != "error" {
		t.Errorf("Expected result 'error', got %s", entry.Result)
	}

	if entry.Error != "test error" {
		t.Errorf("Expected error 'test error', got %s", entry.Error)
	}
}

func TestLogger_LogCommand_SkipCommands(t *testing.T) {
	// Clear any existing history to ensure clean test
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_skip.json"),
		MaxEntries: 100,
	}

	trail, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	logger := &Logger{
		trail: trail,
		skipCommands: map[string]bool{
			"history":     true,
			"undo":        true,
			"interactive": true,
			"i":           true,
			"repl":        true,
		},
	}

	skipCommands := []string{"history", "undo", "interactive", "i", "repl"}

	for _, cmd := range skipCommands {
		args := []string{cmd, "arg1"}

		err = logger.LogCommand(args, nil, time.Second)
		if err != nil {
			t.Errorf("Expected no error for command %s, got %v", cmd, err)
		}
	}

	// Should have no entries logged
	history := logger.trail.GetHistory(10)
	if len(history) != 0 {
		t.Errorf("Expected 0 entries for skip commands, got %d", len(history))
	}
}

func TestLogger_LogCommand_EmptyArgs(t *testing.T) {
	// Create isolated logger for this test
	tmpDir := t.TempDir()
	config := &Config{
		Enabled:    true,
		FilePath:   filepath.Join(tmpDir, "test_empty.json"),
		MaxEntries: 100,
	}

	trail, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create trail: %v", err)
	}

	logger := &Logger{
		trail:        trail,
		skipCommands: make(map[string]bool),
	}

	// Test with empty args
	err = logger.LogCommand([]string{}, nil, time.Second)
	if err != nil {
		t.Errorf("Expected no error for empty args, got %v", err)
	}

	// Should have no entries logged
	history := logger.trail.GetHistory(10)
	if len(history) != 0 {
		t.Errorf("Expected 0 entries for empty args, got %d", len(history))
	}
}

func TestLogger_LogCommand_NilTrail(t *testing.T) {
	logger := &Logger{
		trail:        nil,
		skipCommands: make(map[string]bool),
	}

	args := []string{"test", "command"}

	err := logger.LogCommand(args, nil, time.Second)

	// Should not error with nil trail
	if err != nil {
		t.Errorf("Expected no error with nil trail, got %v", err)
	}
}

func TestBuildEntry(t *testing.T) {
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	args := []string{"account", "create", "--name", "test", "-o", "json"}
	testErr := errors.New("test error")
	duration := 2 * time.Second

	// Test success case
	entry := logger.buildEntry(args, nil, duration)

	if entry.Command != "account" {
		t.Errorf("Expected command 'account', got %s", entry.Command)
	}

	expectedArgs := []string{"create", "--name", "test", "-o", "json"}
	if len(entry.Args) != len(expectedArgs) {
		t.Errorf("Expected %d args, got %d", len(expectedArgs), len(entry.Args))
	}
	for i, expected := range expectedArgs {
		if i < len(entry.Args) && entry.Args[i] != expected {
			t.Errorf("Expected arg[%d] '%s', got '%s'", i, expected, entry.Args[i])
		}
	}

	if entry.Duration != duration {
		t.Errorf("Expected duration %v, got %v", duration, entry.Duration)
	}

	if entry.Result != "success" {
		t.Errorf("Expected result 'success', got %s", entry.Result)
	}

	// Check flags
	if entry.Flags["name"] != "test" {
		t.Errorf("Expected flag 'name' = 'test', got %s", entry.Flags["name"])
	}

	if entry.Flags["o"] != "json" {
		t.Errorf("Expected flag 'o' = 'json', got %s", entry.Flags["o"])
	}

	// Test error case
	errorEntry := logger.buildEntry(args, testErr, duration)

	if errorEntry.Result != "error" {
		t.Errorf("Expected result 'error', got %s", errorEntry.Result)
	}

	if errorEntry.Error != "test error" {
		t.Errorf("Expected error 'test error', got %s", errorEntry.Error)
	}
}

func TestBuildEntry_WithUndo(t *testing.T) {
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	args := []string{"account", "create", "--name", "test"}

	entry := logger.buildEntry(args, nil, time.Second)

	if !entry.Undoable {
		t.Error("Expected entry to be undoable for create command")
	}

	if entry.UndoCommand == "" {
		t.Error("Expected undo command to be set")
	}

	expectedUndo := "mdz account delete --name test"
	if entry.UndoCommand != expectedUndo {
		t.Errorf("Expected undo command '%s', got %s", expectedUndo, entry.UndoCommand)
	}
}

func TestBuildEntry_SingleArg(t *testing.T) {
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	args := []string{"version"}

	entry := logger.buildEntry(args, nil, time.Second)

	if entry.Command != "version" {
		t.Errorf("Expected command 'version', got %s", entry.Command)
	}

	if len(entry.Args) != 0 {
		t.Errorf("Expected no args, got %v", entry.Args)
	}
}

func TestExtractFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name:     "long flags with values",
			args:     []string{"cmd", "--name", "test", "--output", "json"},
			expected: map[string]string{"name": "test", "output": "json"},
		},
		{
			name:     "short flags with values",
			args:     []string{"cmd", "-n", "test", "-o", "json"},
			expected: map[string]string{"n": "test", "o": "json"},
		},
		{
			name:     "boolean flags",
			args:     []string{"cmd", "--verbose", "--debug"},
			expected: map[string]string{"verbose": "true", "debug": "true"},
		},
		{
			name:     "mixed flags",
			args:     []string{"cmd", "--name", "test", "-v", "--output", "json"},
			expected: map[string]string{"name": "test", "v": "true", "output": "json"},
		},
		{
			name:     "no flags",
			args:     []string{"cmd", "arg1", "arg2"},
			expected: map[string]string{},
		},
		{
			name:     "flag at end",
			args:     []string{"cmd", "arg1", "--verbose"},
			expected: map[string]string{"verbose": "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFlags(tt.args)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d flags, got %d", len(tt.expected), len(result))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected flag '%s' to exist", key)
				} else if actualValue != expectedValue {
					t.Errorf("Expected flag '%s' = '%s', got '%s'", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestBuildUndoCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "simple create",
			args:     []string{"account", "create"},
			expected: "mdz account delete",
		},
		{
			name:     "create with flags",
			args:     []string{"account", "create", "--name", "test"},
			expected: "mdz account delete --name test",
		},
		{
			name:     "non-create command",
			args:     []string{"account", "list"},
			expected: "mdz account list",
		},
		{
			name:     "single arg",
			args:     []string{"version"},
			expected: "",
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildUndoCommand(tt.args)

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	// Save original values
	originalUser := os.Getenv("USER")
	originalUsername := os.Getenv("USERNAME")

	// Cleanup function
	defer func() {
		if originalUser != "" {
			os.Setenv("USER", originalUser)
		} else {
			os.Unsetenv("USER")
		}

		if originalUsername != "" {
			os.Setenv("USERNAME", originalUsername)
		} else {
			os.Unsetenv("USERNAME")
		}
	}()

	// Test with USER set
	os.Setenv("USER", "testuser")
	os.Unsetenv("USERNAME")

	user := GetUser()
	if user != "testuser" {
		t.Errorf("Expected user 'testuser', got '%s'", user)
	}

	// Test with USERNAME set (Windows)
	os.Unsetenv("USER")
	os.Setenv("USERNAME", "windowsuser")

	user = GetUser()
	if user != "windowsuser" {
		t.Errorf("Expected user 'windowsuser', got '%s'", user)
	}

	// Test with neither set
	os.Unsetenv("USER")
	os.Unsetenv("USERNAME")

	user = GetUser()
	if user != "" {
		t.Errorf("Expected empty user, got '%s'", user)
	}
}
