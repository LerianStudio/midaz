package output

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/pkg"
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

type Output interface {
	Output() error
}

// FormatAndPrint formats and prints a message indicating the success of an operation,
// with or without style, depending on the color configuration.
func FormatAndPrint(f *factory.Factory, data interface{}, entity, method string) {
	// If we have a non-empty method, treat it as a success message with ID
	if method != "" {
		var msg, methodStyle string

		var id string

		// Try to get ID from data if it's a string
		if strID, ok := data.(string); ok {
			id = strID
		} else {
			// If it's not a success message format, just print the data
			FormatAndPrintJSON(f, data)
			return
		}

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
	} else {
		// Default handling is to print as JSON
		FormatAndPrintJSON(f, data)
	}
}

// FormatAndPrintJSON formats and prints the data as JSON
func FormatAndPrintJSON(f *factory.Factory, data interface{}) {
	if data == nil {
		return
	}

	// Use a more sophisticated JSON formatter in a real implementation
	// For now, just use fmt.Fprintf to print the data
	fmt.Fprintf(f.IOStreams.Out, "%v\n", data)
}

func Printf(w io.Writer, msg string) {
	g := GeneralOutput{Msg: msg, Out: w}
	g.Output()
}

// Errorf prints an error to the given writer.
// It returns any error encountered during printing.
func Errorf(w io.Writer, err error) error {
	e := ErrorOutput{GeneralOutput: GeneralOutput{Out: w}, Err: err}
	return e.Output()
}

// ErrorfWithCategory prints an error to the given writer with a specific category.
// It returns any error encountered during printing.
func ErrorfWithCategory(w io.Writer, err error, category ErrorCategory) error {
	e := ErrorOutput{
		GeneralOutput: GeneralOutput{Out: w},
		Err:           err,
		Category:      category,
	}
	return e.Output()
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

// ErrorCategory represents the type of error for better user experience
type ErrorCategory string

const (
	// ErrorCategoryUser indicates errors caused by user input or configuration
	ErrorCategoryUser ErrorCategory = "User"
	// ErrorCategorySystem indicates errors in the system or environment
	ErrorCategorySystem ErrorCategory = "System"
	// ErrorCategoryAPI indicates errors from API calls
	ErrorCategoryAPI ErrorCategory = "API"
	// ErrorCategoryUnknown is the default category
	ErrorCategoryUnknown ErrorCategory = "Unknown"
)

// ErrorOutput provides structured error output with categorization
type ErrorOutput struct {
	GeneralOutput GeneralOutput
	Err           error
	Category      ErrorCategory
}

// categorizeError determines the error category based on error type
func categorizeError(err error) ErrorCategory {
	if err == nil {
		return ErrorCategoryUnknown
	}

	// Check if it's one of our defined error types
	switch err.(type) {
	case pkg.EntityNotFoundError, pkg.ValidationError, pkg.EntityConflictError:
		return ErrorCategoryUser
	case pkg.HTTPError, pkg.ResponseError:
		return ErrorCategoryAPI
	case pkg.UnauthorizedError, pkg.ForbiddenError:
		return ErrorCategoryUser
	case pkg.InternalServerError, pkg.FailedPreconditionError:
		return ErrorCategorySystem
	default:
		// Check error messages for common patterns
		errMsg := err.Error()
		if strings.Contains(errMsg, "connection refused") || 
		   strings.Contains(errMsg, "timeout") ||
		   strings.Contains(errMsg, "no such file") {
			return ErrorCategorySystem
		}
		if strings.Contains(errMsg, "unauthorized") || 
		   strings.Contains(errMsg, "permission denied") ||
		   strings.Contains(errMsg, "invalid input") {
			return ErrorCategoryUser
		}
		return ErrorCategoryUnknown
	}
}

func (o *ErrorOutput) Output() error {
	if o.Err != nil {
		// Determine category if not already set
		category := o.Category
		if category == "" {
			category = categorizeError(o.Err)
		}

		var prefix string
		var prefixStyle func(a ...interface{}) string

		switch category {
		case ErrorCategoryUser:
			prefix = "User Error"
			prefixStyle = color.New(color.FgYellow, color.Bold).SprintFunc()
		case ErrorCategorySystem:
			prefix = "System Error" 
			prefixStyle = color.New(color.FgRed, color.Bold).SprintFunc()
		case ErrorCategoryAPI:
			prefix = "API Error"
			prefixStyle = color.New(color.FgMagenta, color.Bold).SprintFunc()
		default:
			prefix = "Error"
			prefixStyle = color.New(color.FgRed).SprintFunc()
		}

		_, err := fmt.Fprintf(o.GeneralOutput.Out, "%s: %s\n", prefixStyle(prefix), o.Err.Error())
		if err != nil {
			return err
		}
	}

	return nil
}
