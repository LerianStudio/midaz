package balance

import (
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdBalance(t *testing.T) {
	t.Run("creates balance command with correct attributes", func(t *testing.T) {
		// Setup
		ios := iostreams.System()
		f := &factory.Factory{
			IOStreams: ios,
		}

		// Execute
		cmd := NewCmdBalance(f)

		// Verify
		assert.Equal(t, "balance", cmd.Use)
		assert.Equal(t, "Manages balances within a ledger.", cmd.Short)
		assert.NotEmpty(t, cmd.Long)
		assert.NotEmpty(t, cmd.Example)
		assert.NotNil(t, cmd.Flags().Lookup("help"))

		// Verify subcommands
		subCmds := cmd.Commands()
		assert.Equal(t, 4, len(subCmds))

		// Check each subcommand exists
		var hasListCmd, hasDescribeCmd, hasListByAccountCmd, hasDeleteCmd bool
		for _, subCmd := range subCmds {
			switch subCmd.Use {
			case "list":
				hasListCmd = true
			case "describe":
				hasDescribeCmd = true
			case "list-by-account":
				hasListByAccountCmd = true
			case "delete":
				hasDeleteCmd = true
			}
		}

		assert.True(t, hasListCmd, "list subcommand should exist")
		assert.True(t, hasDescribeCmd, "describe subcommand should exist")
		assert.True(t, hasListByAccountCmd, "list-by-account subcommand should exist")
		assert.True(t, hasDeleteCmd, "delete subcommand should exist")
	})
}

func TestFactoryBalanceSetCmds(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}
	fBalance := &factoryBalance{
		factory: f,
	}
	cmd := &cobra.Command{}

	// Execute
	fBalance.setCmds(cmd)

	// Verify
	subCmds := cmd.Commands()
	assert.Equal(t, 4, len(subCmds))

	// Check each subcommand exists
	var hasListCmd, hasDescribeCmd, hasListByAccountCmd, hasDeleteCmd bool
	for _, subCmd := range subCmds {
		switch subCmd.Use {
		case "list":
			hasListCmd = true
		case "describe":
			hasDescribeCmd = true
		case "list-by-account":
			hasListByAccountCmd = true
		case "delete":
			hasDeleteCmd = true
		}
	}

	assert.True(t, hasListCmd, "list subcommand should exist")
	assert.True(t, hasDescribeCmd, "describe subcommand should exist")
	assert.True(t, hasListByAccountCmd, "list-by-account subcommand should exist")
	assert.True(t, hasDeleteCmd, "delete subcommand should exist")
}
