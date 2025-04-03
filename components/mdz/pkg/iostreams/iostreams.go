package iostreams

import (
	"io"
	"os"
)

// \1 represents an entity
type IOStreams struct {
	In  io.ReadCloser
	Out io.Writer
	Err io.Writer
}

// \1 performs an operation
func System() *IOStreams {
	return &IOStreams{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
}
