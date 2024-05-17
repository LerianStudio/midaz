package pkg

import (
	"github.com/spf13/cobra"
)

// Renderable defines an interface for objects that can render themselves.
// It requires implementing the Render method, which takes a cobra.Command and
// a slice of strings (arguments) and returns an error.
type Renderable interface {
	Render(cmd *cobra.Command, args []string) error
}

// Controller is a generic interface that defines the structure of a controller
// capable of handling commands. It requires two methods:
//   - GetStore, which returns a store of type T.
//   - Run, which takes a cobra.Command and a slice of strings (arguments),
//     and returns a Renderable and an error. This method is responsible for
//     executing the command's logic.
type Controller[T any] interface {
	GetStore() T
	Run(cmd *cobra.Command, args []string) (Renderable, error)
}

// ExportedData represents a generic structure for data that can be exported.
// It contains a single field, Data, which can hold any type of value. The field
// is tagged with `json:"data"` to specify its JSON key when serialized.
type ExportedData struct {
	Data any `json:"data"`
}
