package main

import (
	"os"

	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/root"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
)

func main() {
	env, err := environment.LoadEnv()
	if err != nil {
		output.Printf(os.Stderr, "Failed load envs: "+err.Error())

		os.Exit(1)
	}

	f := factory.NewFactory(&env)
	cmd := root.NewCmdRoot(f)

	if err := cmd.Execute(); err != nil {
		printErr := output.Errorf(f.IOStreams.Err, err)
		if printErr != nil {
			output.Printf(os.Stderr, "Failed to print error output: "+printErr.Error())

			os.Exit(1)
		}

		os.Exit(1)
	}
}
