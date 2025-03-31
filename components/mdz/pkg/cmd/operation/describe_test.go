package operation

import (
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	_ "github.com/LerianStudio/midaz/pkg/mmodel" // Used by mockOperationRepo
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	_ "github.com/stretchr/testify/mock" // Used by mockOperationRepo
)

func TestNewCmdOperationDescribe(t *testing.T) {
	// Setup
	mockRepo := new(mockOperationRepo)
	f := &factoryOperationDescribe{
		repoOperation: mockRepo,
	}

	// Execute
	cmd := newCmdOperationDescribe(f)

	// Verify
	assert.Equal(t, "describe", cmd.Use)
	assert.Equal(t, "Describes an operation.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "operation-id", "output", "help",
	}
	for _, flag := range flags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacDescribe(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacDescribe(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoOperation)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryOperationDescribeRunE(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func(*factoryOperationDescribe, *cobra.Command)
		setupMocks    func(*mockOperationRepo)
		expectedError string
	}{
		{
			name: "successfully describes operation with table output",
			setupFlags: func(f *factoryOperationDescribe, _ *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.OperationID = "op123"
				f.OutputFormat = "table"

				_ = f.OrganizationID
				_ = f.LedgerID
				_ = f.OperationID
				_ = f.OutputFormat
			},
			setupMocks: func(_ *mockOperationRepo) {
				now := time.Now()
				metadata := map[string]interface{}{
					"source": "test",
				}
				_ = now
				_ = metadata
			},
		},
		{
			name: "successfully describes operation with json output",
			setupFlags: func(f *factoryOperationDescribe, _ *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.OperationID = "op123"
				f.OutputFormat = "json"

				_ = f.OrganizationID
				_ = f.LedgerID
				_ = f.OperationID
				_ = f.OutputFormat
			},
			setupMocks: func(_ *mockOperationRepo) {
				now := time.Now()
				metadata := map[string]interface{}{
					"source": "test",
				}
				_ = now
				_ = metadata
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryOperationDescribe, _ *cobra.Command) {
				f.OrganizationID = ""
				f.tuiInput = func(message string) (string, error) {
					return "", errors.New("input error")
				}
			},
			setupMocks: func(_ *mockOperationRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when ledger ID is missing and input fails",
			setupFlags: func(f *factoryOperationDescribe, _ *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(_ *mockOperationRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when operation ID is missing and input fails",
			setupFlags: func(f *factoryOperationDescribe, _ *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.OperationID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					}
					if message == "Enter your ledger-id" {
						return "ledger123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(_ *mockOperationRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when repository call fails",
			setupFlags: func(f *factoryOperationDescribe, _ *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.OperationID = "op123"

				_ = f.OrganizationID
				_ = f.LedgerID
				_ = f.OperationID
			},
			setupMocks: func(mockRepo *mockOperationRepo) {
				mockRepo.On("GetByID", "org123", "ledger123", "op123").Return(
					nil, errors.New("repository error"),
				)
			},
			expectedError: "repository error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ios := iostreams.System()
			mockRepo := new(mockOperationRepo)

			f := &factory.Factory{
				IOStreams: ios,
			}

			facDescribe := &factoryOperationDescribe{
				factory:       f,
				repoOperation: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("operation-id", "", "")
			cmd.Flags().String("output", "table", "")

			// Apply test-specific setup
			tt.setupFlags(facDescribe, cmd)
			tt.setupMocks(mockRepo)

			// Execute
			err := facDescribe.runE(cmd, []string{})

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestFactoryOperationDescribeSetFlags(t *testing.T) {
	// Setup
	f := &factoryOperationDescribe{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "operation-id", "output", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}

	// Verify default values
	assert.Equal(t, "table", cmd.Flag("output").DefValue)
}
