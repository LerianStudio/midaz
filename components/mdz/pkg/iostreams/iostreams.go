package iostreams

import (
	"io"
	"os"
)

type IOStreams struct {
	In  io.ReadCloser
	Out io.Writer
	Err io.Writer
}

func System() *IOStreams {
	return &IOStreams{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
}
