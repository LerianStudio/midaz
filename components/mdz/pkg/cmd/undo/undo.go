package undo

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/LerianStudio/midaz/components/mdz/pkg/audit"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/spf13/cobra"
)

type factoryUndo struct {
	factory *factory.Factory
	last    bool
	dryRun  bool
}

func NewCmdUndo(f *factory.Factory) *cobra.Command {
	fUndo := &factoryUndo{
		factory: f,
	}

	cmd := &cobra.Command{
		Use:   "undo [id]",
		Short: "Undo a previous command",
		Long: `Undo a previously executed command that supports undo operations.

You can undo a specific command by providing its ID, or undo the last
undoable command with the --last flag.

Commands that support undo:
- create operations (undo deletes the created resource)
- update operations (undo reverts to previous state)
- delete operations (undo recreates the resource if possible)`,
		Example: `  mdz undo 1234567890-123456789
  mdz undo --last
  mdz undo --last --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: fUndo.runE,
	}

	fUndo.setFlags(cmd)

	return cmd
}

func (f *factoryUndo) setFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.last, "last", false, "Undo the last undoable command")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Show what would be undone without executing")
}

func (f *factoryUndo) runE(cmd *cobra.Command, args []string) error {
	// Initialize audit trail
	config := audit.DefaultConfig()
	trail, err := audit.New(config)
	if err != nil {
		return fmt.Errorf("failed to initialize audit trail: %w", err)
	}

	var entry *audit.Entry

	if f.last {
		// Get the last undoable command
		undoable := trail.GetUndoableCommands(1)
		if len(undoable) == 0 {
			return fmt.Errorf("no undoable commands found")
		}
		entry = &undoable[0]
	} else {
		// Get specific entry by ID
		if len(args) == 0 {
			return fmt.Errorf("please provide a command ID or use --last flag")
		}

		entry, err = trail.GetEntry(args[0])
		if err != nil {
			return fmt.Errorf("command not found: %s", args[0])
		}

		if !entry.Undoable {
			return fmt.Errorf("command %s is not undoable", args[0])
		}
	}

	// Show what will be undone
	fmt.Fprintf(f.factory.IOStreams.Out, "Undoing command: %s %s\n",
		entry.Command, strings.Join(entry.Args, " "))
	fmt.Fprintf(f.factory.IOStreams.Out, "Executed at: %s\n",
		entry.Timestamp.Format("2006-01-02 15:04:05"))

	if f.dryRun {
		fmt.Fprintln(f.factory.IOStreams.Out, "\nDry run mode - would execute:")
		fmt.Fprintf(f.factory.IOStreams.Out, "  %s\n", entry.UndoCommand)
		return nil
	}

	// Confirm undo
	fmt.Fprintln(f.factory.IOStreams.Out, "\nThis will execute:")
	fmt.Fprintf(f.factory.IOStreams.Out, "  %s\n", entry.UndoCommand)
	fmt.Fprint(f.factory.IOStreams.Out, "\nContinue? (y/N): ")

	var response string
	_, _ = fmt.Fscanln(f.factory.IOStreams.In, &response)
	if strings.ToLower(response) != "y" {
		fmt.Fprintln(f.factory.IOStreams.Out, "Undo cancelled")
		return nil
	}

	// Execute undo command
	fmt.Fprintln(f.factory.IOStreams.Out, "\nExecuting undo...")

	// Parse undo command
	parts := strings.Fields(entry.UndoCommand)
	if len(parts) == 0 {
		return fmt.Errorf("invalid undo command")
	}

	// Execute the command
	execCmd := exec.Command(parts[0], parts[1:]...)
	execCmd.Stdout = f.factory.IOStreams.Out
	execCmd.Stderr = f.factory.IOStreams.Err
	execCmd.Stdin = f.factory.IOStreams.In

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("undo failed: %w", err)
	}

	// Log the undo operation
	undoEntry := audit.NewBuilder().
		WithCommand("undo").
		WithArgs([]string{entry.ID}).
		WithResult("success").
		WithMetadata("undid_command", entry.Command).
		WithMetadata("undid_id", entry.ID).
		Build()

	if err := trail.LogCommand(undoEntry); err != nil {
		// Don't fail the operation, just warn
		fmt.Fprintf(f.factory.IOStreams.Err, "Warning: failed to log undo operation: %v\n", err)
	}

	fmt.Fprintln(f.factory.IOStreams.Out, "\nUndo completed successfully")
	return nil
}
