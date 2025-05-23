package audit

import (
	"os"
	"strings"
	"time"
)

// Logger handles command logging for audit trail
type Logger struct {
	trail      *Trail
	skipCommands map[string]bool
}

// NewLogger creates a new audit logger
func NewLogger() (*Logger, error) {
	config := DefaultConfig()
	trail, err := New(config)
	if err != nil {
		return nil, err
	}
	
	return &Logger{
		trail: trail,
		skipCommands: map[string]bool{
			"history":     true,
			"undo":        true,
			"interactive": true,
			"i":           true,
			"repl":        true,
		},
	}, nil
}

// LogCommand logs a command execution to the audit trail
func (l *Logger) LogCommand(args []string, err error, duration time.Duration) error {
	if l.trail == nil || len(args) == 0 {
		return nil
	}
	
	// Skip certain commands
	if len(args) > 0 && l.skipCommands[args[0]] {
		return nil
	}
	
	entry := l.buildEntry(args, err, duration)
	return l.trail.LogCommand(entry)
}

// buildEntry builds an audit entry from command arguments
func (l *Logger) buildEntry(args []string, err error, duration time.Duration) Entry {
	builder := NewBuilder().
		WithCommand(args[0]).
		WithDuration(duration)
	
	if len(args) > 1 {
		builder.WithArgs(args[1:])
	}
	
	// Extract flags
	flags := extractFlags(args)
	builder.WithFlags(flags)
	
	// Set result
	if err != nil {
		builder.WithError(err)
	} else {
		builder.WithResult("success")
		
		// Add undo information for create/delete commands
		if len(args) > 1 && args[1] == "create" {
			undoCmd := buildUndoCommand(args)
			if undoCmd != "" {
				builder.WithUndo(undoCmd)
			}
		}
	}
	
	return builder.Build()
}

// extractFlags extracts flags from command arguments
func extractFlags(args []string) map[string]string {
	flags := make(map[string]string)
	
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			value := "true"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				value = args[i+1]
				i++
			}
			flags[key] = value
		} else if strings.HasPrefix(args[i], "-") && len(args[i]) == 2 {
			key := string(args[i][1])
			value := "true"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				value = args[i+1]
				i++
			}
			flags[key] = value
		}
	}
	
	return flags
}

// buildUndoCommand builds an undo command for create operations
func buildUndoCommand(args []string) string {
	if len(args) < 2 {
		return ""
	}
	
	cmdStr := strings.Join(args, " ")
	return "mdz " + strings.Replace(cmdStr, "create", "delete", 1)
}

// GetUser returns the current user
func GetUser() string {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME") // Windows
	}
	return user
}