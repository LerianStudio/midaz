// Package output provides formatted output utilities for the MDZ CLI.
//
// This package handles CLI output formatting with support for:
//   - Colored output for better readability
//   - Success messages with entity information
//   - Error messages with consistent formatting
//   - No-color mode for CI/CD environments
package output

import (
	"fmt"
	"io"
	"log"

	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"
	"github.com/fatih/color"
)

// Operation status constants for output formatting.
const (
	Created = "created" // Entity was created
	Deleted = "deleted" // Entity was deleted
	Updated = "updated" // Entity was updated
)

var (
	// IDs in pink and bold
	idStyle = color.New(color.FgHiMagenta, color.Bold).SprintFunc()

	// Confirmations: "Y" in green and "N" in red
	// yesStyle = color.New(color.FgGreen).SprintFunc()
	// noStyle  = color.New(color.FgRed).SprintFunc()

	// entitys in blue and bold
	entityStyle = color.New(color.FgBlue, color.Bold).SprintFunc()

	// HTTP methods in specific colors
	postStyle   = color.New(color.FgYellow, color.Bold).SprintFunc()
	deleteStyle = color.New(color.FgRed, color.Bold).SprintFunc()
	// putStyle    = color.New(color.FgBlue, color.Bold).SprintFunc()
	patchStyle = color.New(color.FgBlue, color.Bold).SprintFunc()
	// getStyle    = color.New(color.FgGreen, color.Bold).SprintFunc()
)

// Output is an interface for types that can output themselves.
type Output interface {
	Output() error
}

// FormatAndPrint formats and prints a success message for CLI operations.
//
// This function creates user-friendly success messages with optional colored output:
//   - Colored mode: Uses colored text with checkmark (✔︎)
//   - No-color mode: Plain text without colors
//
// Message format: "The {entity} {id} has been successfully {method}."
//
// Parameters:
//   - f: Factory with IOStreams and NoColor flag
//   - id: Entity ID (e.g., UUID)
//   - entity: Entity type (e.g., "organization", "account")
//   - method: Operation performed (Created, Updated, Deleted)
func FormatAndPrint(f *factory.Factory, id, entity, method string) {
	var msg, methodStyle string

	if f.NoColor {
		msg = fmt.Sprintf("The %s %s has been successfully %s.",
			entity,
			id,
			method,
		)
	} else {
		switch method {
		case Created:
			methodStyle = postStyle(method)
		case Deleted:
			methodStyle = deleteStyle(method)
		case Updated:
			methodStyle = patchStyle(method)
		default:
			methodStyle = method
		}

		msg = fmt.Sprintf("✔︎  The %s %s has been successfully %s.",
			entityStyle(entity),
			idStyle(id),
			methodStyle,
		)
	}

	g := GeneralOutput{Msg: msg, Out: f.IOStreams.Out}
	g.Output()
}

// Printf prints a message to the specified writer.
//
// Parameters:
//   - w: Writer to output to
//   - msg: Message to print
func Printf(w io.Writer, msg string) {
	g := GeneralOutput{Msg: msg, Out: w}
	g.Output()
}

// Errorf prints an error message to the specified writer.
//
// Parameters:
//   - w: Writer to output error to
//   - err: Error to format and print
//
// Returns:
//   - error: Error if writing fails
func Errorf(w io.Writer, err error) error {
	e := ErrorOutput{GeneralOutput: GeneralOutput{Out: w}, Err: err}

	return e.Output()
}

// GeneralOutput represents a general message output.
type GeneralOutput struct {
	Msg string    // Message to output
	Out io.Writer // Writer to output to
}

// Output writes the message to the output stream.
func (o *GeneralOutput) Output() {
	if _, err := fmt.Fprintf(o.Out, "%s\n", o.Msg); err != nil {
		log.Printf("failed to write output: %v", err)
	}
}

// ErrorOutput represents an error message output.
type ErrorOutput struct {
	GeneralOutput GeneralOutput // General output configuration
	Err           error         // Error to output
}

// Output writes the error message to the output stream.
//
// Returns:
//   - error: Error if writing fails, nil otherwise
func (o *ErrorOutput) Output() error {
	if o.Err != nil {
		_, err := fmt.Fprintf(o.GeneralOutput.Out, "Error: %s\n", o.Err.Error())
		if err != nil {
			return err
		}
	}

	return nil
}
