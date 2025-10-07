// Package segment provides CLI commands for managing segments.
//
// This package implements the "mdz segment" command group with subcommands
// for create, list, describe, update, and delete operations.
package segment

import (
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

// factorySegment wraps the factory for segment commands.
type factorySegment struct {
	factory *factory.Factory
}

// setCmds registers all segment subcommands.
func (f *factorySegment) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdSegmentCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdSegmentList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdSegmentDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdSegmentUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdSegmentDelete(newInjectFacDelete(f.factory)))
}

// NewCmdSegment creates the "segment" command with all subcommands.
//
// Returns a Cobra command configured with create, list, describe, update, and delete subcommands.
func NewCmdSegment(f *factory.Factory) *cobra.Command {
	fOrg := factorySegment{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "segment",
		Short: "Shows details of a specific segment.",
		Long: utils.Format(
			"The describe subcommand provides a detailed view of a specific",
			"segment, including its segmenting policies and the rules applied.",
			"Ideal for detailed analysis and monitoring of the settings governing",
			"the grouping of customers within each segment.",
		),
		Example: utils.Format(
			"$ mdz segment",
			"$ mdz segment -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fOrg.setCmds(cmd)

	return cmd
}
