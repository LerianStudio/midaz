package output

import (
	"fmt"
	"io"
	"log"
)

type Output interface {
	Output() error
}

func Printf(w io.Writer, msg string) {
	g := GeneralOutput{Msg: msg, Out: w}
	g.Output()
}

func Errorf(w io.Writer, err error) error {
	e := ErrorOutput{GeneralOutput: GeneralOutput{Out: w}, Err: err}

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

type ErrorOutput struct {
	GeneralOutput GeneralOutput
	Err           error
}

func (o *ErrorOutput) Output() error {
	if o.Err != nil {
		_, err := fmt.Fprintf(o.GeneralOutput.Out, "Error: %s\n", o.Err.Error())
		if err != nil {
			return err
		}
	}

	return nil
}
