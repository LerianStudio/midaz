package main

import (
	"fmt"
	"os"

	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/root"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
)

// exitWithError handles error printing and terminates the program with exit code 1
func exitWithError(f *factory.Factory, err error) {
	printErr := output.Errorf(f.IOStreams.Err, err)
	if printErr != nil {
		// If we can't print to the configured error stream, try stderr directly
		fmt.Fprintf(os.Stderr, "Failed to print error output: %s\n", printErr.Error())
	}
	os.Exit(1)
}

func main() {
	env := environment.New()

	f := factory.NewFactory(env)
	cmd := root.NewCmdRoot(f)

	if err := cmd.Execute(); err != nil {
		exitWithError(f, err)
	}
}
