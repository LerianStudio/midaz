package transaction

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdTransactionCreateDSL(t *testing.T) {
	// Setup
	mockRepo := new(mockTransactionRepo)
	f := &factoryTransactionCreateDSL{
		repoTransaction: mockRepo,
	}

	// Execute
	cmd := newCmdTransactionCreateDSL(f)

	// Verify
	assert.Equal(t, "create-dsl", cmd.Use)
	assert.Equal(t, "Creates a transaction using DSL.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	expectedFlags := []string{
		"organization-id", "ledger-id", "dsl-file", "help",
	}
	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacCreateDSL(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacCreateDSL(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoTransaction)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryTransactionCreateDSLRunE(t *testing.T) {
	// Create a temporary file for testing
	tempDSLContent := "transaction { description: 'Test DSL transaction' }"
	tempFile, err := ioutil.TempFile("", "dsl-test-*.dsl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write([]byte(tempDSLContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	tests := []struct {
		name          string
		setupFlags    func(*factoryTransactionCreateDSL, *cobra.Command)
		setupMocks    func(*mockTransactionRepo)
		setupStdin    func() *os.File
		expectedError string
		cleanup       func()
	}{
		{
			name: "successfully creates transaction from DSL file",
			setupFlags: func(f *factoryTransactionCreateDSL, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.DSLFile = tempFile.Name()

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("dsl-file", f.DSLFile)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				mockRepo.On("CreateDSL", "org123", "ledger123", tempDSLContent).Return(
					&mmodel.Transaction{
						ID:          "tx123",
						Description: "Test DSL transaction",
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					}, nil,
				)
			},
			setupStdin: func() *os.File { return nil },
			cleanup:    func() {},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryTransactionCreateDSL, cmd *cobra.Command) {
				f.OrganizationID = ""
				f.tuiInput = func(message string) (string, error) {
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			setupStdin:    func() *os.File { return nil },
			expectedError: "input error",
			cleanup:       func() {},
		},
		{
			name: "fails when ledger ID is missing and input fails",
			setupFlags: func(f *factoryTransactionCreateDSL, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			setupStdin:    func() *os.File { return nil },
			expectedError: "input error",
			cleanup:       func() {},
		},
		{
			name: "fails when DSL file is missing and input fails",
			setupFlags: func(f *factoryTransactionCreateDSL, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.DSLFile = ""
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
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			setupStdin:    func() *os.File { return nil },
			expectedError: "input error",
			cleanup:       func() {},
		},
		{
			name: "fails when DSL file does not exist",
			setupFlags: func(f *factoryTransactionCreateDSL, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.DSLFile = "nonexistent-file.dsl"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("dsl-file", f.DSLFile)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			setupStdin:    func() *os.File { return nil },
			expectedError: "reading DSL file",
			cleanup:       func() {},
		},
		{
			name: "fails when repository call fails",
			setupFlags: func(f *factoryTransactionCreateDSL, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.DSLFile = tempFile.Name()

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("dsl-file", f.DSLFile)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				mockRepo.On("CreateDSL", "org123", "ledger123", tempDSLContent).Return(
					nil, errors.New("repository error"),
				)
			},
			setupStdin:    func() *os.File { return nil },
			expectedError: "repository error",
			cleanup:       func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ios := iostreams.System()
			mockRepo := new(mockTransactionRepo)

			f := &factory.Factory{
				IOStreams: ios,
			}

			facCreateDSL := &factoryTransactionCreateDSL{
				factory:         f,
				repoTransaction: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("dsl-file", "", "")

			// Apply test-specific setup
			tt.setupFlags(facCreateDSL, cmd)
			tt.setupMocks(mockRepo)

			// Setup stdin if needed
			origStdin := os.Stdin
			if stdin := tt.setupStdin(); stdin != nil {
				os.Stdin = stdin
				defer func() { os.Stdin = origStdin }()
			}

			// Execute
			err := facCreateDSL.runE(cmd, []string{})

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)

			// Cleanup
			tt.cleanup()
		})
	}
}

func TestFactoryTransactionCreateDSLSetFlags(t *testing.T) {
	// Setup
	f := &factoryTransactionCreateDSL{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "dsl-file", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}
