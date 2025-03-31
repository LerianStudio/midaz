package transaction

import (
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdTransaction(t *testing.T) {
	t.Run("creates transaction command with correct attributes", func(t *testing.T) {
		// Setup
		ios := iostreams.System()
		f := &factory.Factory{
			IOStreams: ios,
		}

		// Execute
		cmd := NewCmdTransaction(f)

		// Verify
		assert.Equal(t, "transaction", cmd.Use)
		assert.Equal(t, "Manages transactions within a ledger.", cmd.Short)
		assert.NotEmpty(t, cmd.Long)
		assert.NotEmpty(t, cmd.Example)
		assert.NotNil(t, cmd.Flags().Lookup("help"))

		// Verify subcommands
		subCmds := cmd.Commands()
		assert.Equal(t, 5, len(subCmds))

		// Check each subcommand exists
		var hasCreateCmd, hasCreateDSLCmd, hasListCmd, hasDescribeCmd, hasRevertCmd bool
		for _, subCmd := range subCmds {
			switch subCmd.Use {
			case "create":
				hasCreateCmd = true
			case "create-dsl":
				hasCreateDSLCmd = true
			case "list":
				hasListCmd = true
			case "describe":
				hasDescribeCmd = true
			case "revert":
				hasRevertCmd = true
			}
		}

		assert.True(t, hasCreateCmd, "create subcommand should exist")
		assert.True(t, hasCreateDSLCmd, "create-dsl subcommand should exist")
		assert.True(t, hasListCmd, "list subcommand should exist")
		assert.True(t, hasDescribeCmd, "describe subcommand should exist")
		assert.True(t, hasRevertCmd, "revert subcommand should exist")
	})
}

func TestFactoryTransactionSetCmds(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}
	fTransaction := &factoryTransaction{
		factory: f,
	}
	cmd := &cobra.Command{}

	// Execute
	fTransaction.setCmds(cmd)

	// Verify
	subCmds := cmd.Commands()
	assert.Equal(t, 5, len(subCmds))

	// Check each subcommand exists
	var hasCreateCmd, hasCreateDSLCmd, hasListCmd, hasDescribeCmd, hasRevertCmd bool
	for _, subCmd := range subCmds {
		switch subCmd.Use {
		case "create":
			hasCreateCmd = true
		case "create-dsl":
			hasCreateDSLCmd = true
		case "list":
			hasListCmd = true
		case "describe":
			hasDescribeCmd = true
		case "revert":
			hasRevertCmd = true
		}
	}

	assert.True(t, hasCreateCmd, "create subcommand should exist")
	assert.True(t, hasCreateDSLCmd, "create-dsl subcommand should exist")
	assert.True(t, hasListCmd, "list subcommand should exist")
	assert.True(t, hasDescribeCmd, "describe subcommand should exist")
	assert.True(t, hasRevertCmd, "revert subcommand should exist")
}
