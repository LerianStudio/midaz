package main

import (
	"os"
	"strings"
	"time"

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
	trail, auditErr := initializeAuditTrail()

	// Execute command
	err := cmd.Execute()
	duration := time.Since(startTime)

	// Log to audit trail if initialized
	if auditErr == nil {
		performAuditLogging(trail, cmdStr, err, duration)
	}

	// Handle command execution error
	handleCommandError(f, err)
}

// handleCommandError handles command execution errors
func handleCommandError(f *factory.Factory, err error) {
	if err != nil {
		printErr := output.Errorf(f.IOStreams.Err, err)
		if printErr != nil {
			output.Printf(os.Stderr, "Failed to print error output: "+printErr.Error())
			os.Exit(1)
		}

		os.Exit(1)
	}
}
