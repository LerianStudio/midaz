package output

import (
	"fmt"
	"io"
	"log"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/fatih/color"
)

const (
	Created = "created"

	Deleted = "deleted"

	Updated = "updated"
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

// \1 represents an entity
type Output interface {
	Output() error
}

// FormatAndPrint formats and prints a message indicating the success of an operation,
// with or without style, depending on the color configuration.
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

// \1 performs an operation
func Printf(w io.Writer, msg string) {
	g := GeneralOutput{Msg: msg, Out: w}
	g.Output()
}

// \1 performs an operation
func Errorf(w io.Writer, err error) error {
	e := ErrorOutput{GeneralOutput: GeneralOutput{Out: w}, Err: err}

	return e.Output()
}

// \1 represents an entity
type GeneralOutput struct {
	Msg string
	Out io.Writer
}

// func (o *GeneralOutput) Output() { performs an operation
func (o *GeneralOutput) Output() {
	if _, err := fmt.Fprintf(o.Out, "%s\n", o.Msg); err != nil {
		log.Printf("failed to write output: %v", err)
	}
}

// \1 represents an entity
type ErrorOutput struct {
	GeneralOutput GeneralOutput
	Err           error
}

// func (o *ErrorOutput) Output() error { performs an operation
func (o *ErrorOutput) Output() error {
	if o.Err != nil {
		_, err := fmt.Fprintf(o.GeneralOutput.Out, "Error: %s\n", o.Err.Error())

		if err != nil {
			return err
		}
	}

	return nil
}
