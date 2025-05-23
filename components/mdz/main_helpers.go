package main

import (
	"os"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/audit"
)

// initializeAuditTrail sets up audit logging
func initializeAuditTrail() (*audit.Trail, error) {
	auditConfig := audit.DefaultConfig()
	return audit.New(auditConfig)
}

// shouldSkipAuditLogging determines if a command should be skipped from audit logs
func shouldSkipAuditLogging(args []string) bool {
	if len(args) == 0 {
		return true
	}

	skipCommands := map[string]bool{
		"history":     true,
		"undo":        true,
		"interactive": true,
		"i":           true,
		"repl":        true,
		"help":        true,
		"--help":      true,
		"-h":          true,
		"version":     true,
		"--version":   true,
	}

	return skipCommands[args[0]]
}

// performAuditLogging logs command execution to audit trail
func performAuditLogging(trail *audit.Trail, cmdStr string, err error, duration time.Duration) {
	if trail == nil || len(os.Args) <= 1 {
		return
	}

	if shouldSkipAuditLogging(os.Args[1:]) {
		return
	}

	entry := audit.NewBuilder().
		WithCommand(os.Args[1]).
		WithDuration(duration)

	if len(os.Args) > 2 {
		entry.WithArgs(os.Args[2:])
	}

	// Extract flags
	flags := extractFlags()
	entry.WithFlags(flags)

	// Set result
	if err != nil {
		entry.WithError(err)
	} else {
		entry.WithResult("success")
		addUndoInformation(entry, cmdStr)
	}

	// Log the command
	_ = trail.LogCommand(entry.Build())
}

// extractFlags parses command line flags
func extractFlags() map[string]string {
	flags := make(map[string]string)

	for i := 1; i < len(os.Args); i++ {
		if strings.HasPrefix(os.Args[i], "--") {
			key := strings.TrimPrefix(os.Args[i], "--")

			value := "true"
			if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				value = os.Args[i+1]
				i++
			}

			flags[key] = value
		} else if strings.HasPrefix(os.Args[i], "-") && len(os.Args[i]) == 2 {
			key := string(os.Args[i][1])

			value := "true"
			if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				value = os.Args[i+1]
				i++
			}

			flags[key] = value
		}
	}

	return flags
}

// addUndoInformation adds undo commands for reversible operations
func addUndoInformation(entry *audit.Builder, cmdStr string) {
	if len(os.Args) <= 2 {
		return
	}

	switch os.Args[2] {
	case "create":
		// For create commands, undo would be delete
		if len(os.Args) > 3 {
			undoCmd := strings.Replace(cmdStr, "create", "delete", 1)
			entry.WithUndo("mdz " + undoCmd)
		}
	case "delete":
		// Delete operations are not easily undoable without the full object
		// Could be enhanced to store the deleted object for recreation
		return
	case "update":
		// Update operations would need to store the previous state
		// This could be enhanced with state snapshots
		return
	}
}
