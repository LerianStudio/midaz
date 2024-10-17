package main

import (
	"fmt"
	"os"

	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/root"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
)

func main() {
	f := factory.NewFactory()
	cmd := root.NewCmdRoot(f)

	if err := cmd.Execute(); err != nil {
		printErr := output.Print(&output.ErrorOutput{
			GeneralOutput: output.GeneralOutput{
				Out: f.IOStreams.Err,
			},
			Err: err,
		})
		if printErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to print error output: %v\n", printErr)
			os.Exit(1)
		}
	}
}
