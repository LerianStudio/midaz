// Package iostreams provides I/O stream abstractions for the MDZ CLI.
//
// This package wraps standard input, output, and error streams, making them
// injectable for testing and allowing commands to be tested without actual
// terminal I/O.
package iostreams

import (
	"io"
	"os"
)

// IOStreams holds input, output, and error streams for CLI operations.
//
// This struct provides abstraction over I/O streams, enabling:
//   - Testability: Mock streams for unit tests
//   - Flexibility: Redirect output to files or buffers
//   - Consistency: Centralized I/O management
type IOStreams struct {
	In  io.ReadCloser // Input stream (typically stdin)
	Out io.Writer     // Output stream (typically stdout)
	Err io.Writer     // Error stream (typically stderr)
}

// System creates an IOStreams instance using system streams.
//
// This function returns an IOStreams with:
//   - In: os.Stdin
//   - Out: os.Stdout
//   - Err: os.Stderr
//
// Returns:
//   - *IOStreams: IOStreams connected to system streams
func System() *IOStreams {
	return &IOStreams{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
}
