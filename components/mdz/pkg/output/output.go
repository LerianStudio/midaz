package output

import (
	"fmt"
	"io"
	"log"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
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

		msg = fmt.Sprintf("The %s %s has been successfully %s.",
			entityStyle(entity),
			idStyle(id),
			methodStyle,
		)
	}

	g := GeneralOutput{Msg: msg, Out: f.IOStreams.Out}
	g.Output()
}

// Printf prints a formatted message to the writer.
func Printf(w io.Writer, format string, a ...any) {
	g := GeneralOutput{Msg: fmt.Sprintf(format, a...), Out: w}
	g.Output()
}

// Errorf prints an error message to the writer and returns the error.
func Errorf(w io.Writer, err error) error {
	e := ErrorOutput{GeneralOutput: GeneralOutput{Out: w}, Err: err}

	return e.Output()
}

// TableWriter is a wrapper around tablewriter.Table to provide backwards compatibility
type TableWriter struct {
	*tablewriter.Table
}

// SetHeader sets the table header
func (t *TableWriter) SetHeader(headers []string) {
	t.Header(headers)
}

// Append adds a row to the table
func (t *TableWriter) Append(row []string) {
	_ = t.Table.Append(row)
}

// Render renders the table
func (t *TableWriter) Render() {
	_ = t.Table.Render()
}

// NewTable creates a new table writer.
func NewTable(w io.Writer) *TableWriter {
	return &TableWriter{
		Table: tablewriter.NewWriter(w),
	}
}

type GeneralOutput struct {
	Msg string
	Out io.Writer
}

func (o *GeneralOutput) Output() {
	if _, err := fmt.Fprintf(o.Out, "%s\n", o.Msg); err != nil {
		log.Printf("failed to write output: %v", err)
	}
}

type ErrorOutput struct {
	GeneralOutput
	Err error
}

func (o *ErrorOutput) Output() error {
	log.Println(o.Err)

	if _, err := fmt.Fprintf(o.Out, "%s\n", o.Err); err != nil {
		log.Printf("failed to write error output: %v", err)
	}

	return o.Err
}
