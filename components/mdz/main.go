package main

import (
	"os"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/audit"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/root"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
)

func main() {
	startTime := time.Now()
	env := environment.New()

	f := factory.NewFactory(env)
	cmd := root.NewCmdRoot(f)

	// Capture command for audit logging
	cmdStr := strings.Join(os.Args[1:], " ")

	// Initialize audit trail
	auditConfig := audit.DefaultConfig()
	trail, auditErr := audit.New(auditConfig)

	// Execute command
	err := cmd.Execute()
	duration := time.Since(startTime)

	// Log to audit trail if initialized
	if auditErr == nil && trail != nil && len(os.Args) > 1 {
		// Don't log history, undo, or interactive commands to avoid recursion
		skipCommands := map[string]bool{
			"history":     true,
			"undo":        true,
			"interactive": true,
			"i":           true,
			"repl":        true,
		}

		if len(os.Args) > 1 && !skipCommands[os.Args[1]] {
			entry := audit.NewBuilder().
				WithCommand(os.Args[1]).
				WithDuration(duration)

			if len(os.Args) > 2 {
				entry.WithArgs(os.Args[2:])
			}

			// Extract flags
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
			entry.WithFlags(flags)

			// Set result
			if err != nil {
				entry.WithError(err)
			} else {
				entry.WithResult("success")

				// Add undo information for create/delete commands
				if len(os.Args) > 2 {
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
					case "update":
						// Update operations would need to store the previous state
						// This could be enhanced with state snapshots
					}
				}
			}

			// Log the command
			_ = trail.LogCommand(entry.Build())
		}
	}

	// Handle command execution error
	if err != nil {
		printErr := output.Errorf(f.IOStreams.Err, err)
		if printErr != nil {
			output.Printf(os.Stderr, "Failed to print error output: "+printErr.Error())
			os.Exit(1)
		}
		os.Exit(1)
	}
}
