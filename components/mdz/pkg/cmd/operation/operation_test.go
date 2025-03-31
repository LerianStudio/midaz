package operation

import (
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdOperation(t *testing.T) {
	t.Run("creates operation command with correct attributes", func(t *testing.T) {
		// Setup
		ios := iostreams.System()
		f := &factory.Factory{
			IOStreams: ios,
		}

		// Execute
		cmd := NewCmdOperation(f)

		// Verify
		assert.Equal(t, "operation", cmd.Use)
		assert.Equal(t, "Manages operations within a ledger.", cmd.Short)
		assert.NotEmpty(t, cmd.Long)
		assert.NotEmpty(t, cmd.Example)
		assert.NotNil(t, cmd.Flags().Lookup("help"))

		// Verify subcommands
		subCmds := cmd.Commands()
		assert.Equal(t, 3, len(subCmds))

		// Check each subcommand exists
		var hasListCmd, hasDescribeCmd, hasListByAccountCmd bool
		for _, subCmd := range subCmds {
			switch subCmd.Use {
			case "list":
				hasListCmd = true
			case "describe":
				hasDescribeCmd = true
			case "list-by-account":
				hasListByAccountCmd = true
			}
		}

		assert.True(t, hasListCmd, "list subcommand should exist")
		assert.True(t, hasDescribeCmd, "describe subcommand should exist")
		assert.True(t, hasListByAccountCmd, "list-by-account subcommand should exist")
	})
}

func TestFactoryOperationSetCmds(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}
	fOperation := &factoryOperation{
		factory: f,
	}
	cmd := &cobra.Command{}

	// Execute
	fOperation.setCmds(cmd)

	// Verify
	subCmds := cmd.Commands()
	assert.Equal(t, 3, len(subCmds))

	// Check each subcommand exists
	var hasListCmd, hasDescribeCmd, hasListByAccountCmd bool
	for _, subCmd := range subCmds {
		switch subCmd.Use {
		case "list":
			hasListCmd = true
		case "describe":
			hasDescribeCmd = true
		case "list-by-account":
			hasListByAccountCmd = true
		}
	}

	assert.True(t, hasListCmd, "list subcommand should exist")
	assert.True(t, hasDescribeCmd, "describe subcommand should exist")
	assert.True(t, hasListByAccountCmd, "list-by-account subcommand should exist")
}
