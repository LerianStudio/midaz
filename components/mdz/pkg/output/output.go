package output

import (
	"fmt"
	"io"
)

type Output interface {
	Output() error
}

func Print(out Output) error {
	return out.Output()
}

type GeneralOutput struct {
	Msg string
	Out io.Writer
}

func (o *GeneralOutput) Output() error {
	if _, err := fmt.Fprintf(o.Out, "%s\n", o.Msg); err != nil {
		return err
	}
	return nil
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
