package history

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/audit"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/spf13/cobra"
)

type factoryHistory struct {
	factory      *factory.Factory
	limit        int
	showUndoable bool
	clear        bool
	format       string
}

func NewCmdHistory(f *factory.Factory) *cobra.Command {
	fHistory := &factoryHistory{
		factory: f,
		limit:   50,
		format:  "table",
	}

	cmd := &cobra.Command{
		Use:   "history",
		Short: "View command history and audit trail",
		Long: `View the history of commands executed with MDZ.

The history command shows:
- Command and arguments executed
- Timestamp of execution
- Duration of command
- Success or error status
- Undo commands for reversible operations`,
		Example: `  mdz history
  mdz history --limit 100
  mdz history --undoable
  mdz history --format json
  mdz history --clear`,
		RunE: fHistory.runE,
	}

	fHistory.setFlags(cmd)

	return cmd
}

func (f *factoryHistory) setFlags(cmd *cobra.Command) {
	cmd.Flags().IntVar(&f.limit, "limit", 50, "Maximum number of entries to show")
	cmd.Flags().BoolVar(&f.showUndoable, "undoable", false, "Show only undoable commands")
	cmd.Flags().BoolVar(&f.clear, "clear", false, "Clear command history")
	cmd.Flags().StringVar(&f.format, "format", "table", "Output format (table, json)")
}

func (f *factoryHistory) runE(cmd *cobra.Command, args []string) error {
	// Initialize audit trail
	config := audit.DefaultConfig()
	trail, err := audit.New(config)
	if err != nil {
		return fmt.Errorf("failed to initialize audit trail: %w", err)
	}

	// Handle clear flag
	if f.clear {
		if err := trail.Clear(); err != nil {
			return fmt.Errorf("failed to clear history: %w", err)
		}
		fmt.Fprintln(f.factory.IOStreams.Out, "Command history cleared successfully")
		return nil
	}

	// Get entries
	var entries []audit.Entry
	if f.showUndoable {
		entries = trail.GetUndoableCommands(f.limit)
	} else {
		entries = trail.GetHistory(f.limit)
	}

	if len(entries) == 0 {
		fmt.Fprintln(f.factory.IOStreams.Out, "No command history found")
		return nil
	}

	// Format and display
	switch f.format {
	case "json":
		// Use JSON marshaling for JSON output
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal history: %w", err)
		}
		fmt.Fprintln(f.factory.IOStreams.Out, string(data))
		return nil
	default:
		return f.printTable(entries)
	}
}

func (f *factoryHistory) printTable(entries []audit.Entry) error {
	out := f.factory.IOStreams.Out

	// Create table
	table := output.NewTable(out)
	table.SetHeader([]string{"ID", "Time", "Command", "Duration", "Status", "Undo"})

	for _, entry := range entries {
		// Format command with args
		cmdStr := entry.Command
		if len(entry.Args) > 0 {
			cmdStr += " " + strings.Join(entry.Args, " ")
		}
		if len(cmdStr) > 50 {
			cmdStr = cmdStr[:47] + "..."
		}

		// Format time
		timeStr := entry.Timestamp.Format("2006-01-02 15:04:05")

		// Format duration
		durationStr := formatDuration(entry.Duration)

		// Format status
		statusStr := entry.Result
		if entry.Error != "" {
			statusStr = "error"
		}

		// Format undo
		undoStr := ""
		if entry.Undoable && entry.UndoCommand != "" {
			undoStr = "✓"
		}

		table.Append([]string{
			entry.ID,
			timeStr,
			cmdStr,
			durationStr,
			statusStr,
			undoStr,
		})
	}

	table.Render()

	// Show summary
	fmt.Fprintf(out, "\nShowing %d of %d entries\n", len(entries), len(entries))

	if f.showUndoable {
		fmt.Fprintln(out, "\nTip: Use 'mdz undo <id>' to undo a command")
	}

	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	} else if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
}
