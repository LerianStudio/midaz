package asset_rate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementation of the AssetRate repository interface
type mockAssetRateRepo struct {
	mock.Mock
}

func (m *mockAssetRateRepo) Create(organizationID, ledgerID string, input mmodel.CreateAssetRateInput) (*mmodel.AssetRate, error) {
	args := m.Called(organizationID, ledgerID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.AssetRate), args.Error(1)
}

func (m *mockAssetRateRepo) Update(organizationID, ledgerID, assetRateID string, input mmodel.UpdateAssetRateInput) (*mmodel.AssetRate, error) {
	args := m.Called(organizationID, ledgerID, assetRateID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.AssetRate), args.Error(1)
}

func (m *mockAssetRateRepo) Get(organizationID, ledgerID string, limit, page int, sortOrder, startDate, endDate string) (*mmodel.AssetRates, error) {
	args := m.Called(organizationID, ledgerID, limit, page, sortOrder, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.AssetRates), args.Error(1)
}

func (m *mockAssetRateRepo) GetByID(organizationID, ledgerID, assetRateID string) (*mmodel.AssetRate, error) {
	args := m.Called(organizationID, ledgerID, assetRateID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.AssetRate), args.Error(1)
}

func (m *mockAssetRateRepo) GetByAssetCode(organizationID, ledgerID, assetCode string, limit, page int, sortOrder, startDate, endDate string) (*mmodel.AssetRates, error) {
	args := m.Called(organizationID, ledgerID, assetCode, limit, page, sortOrder, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mmodel.AssetRates), args.Error(1)
}

func TestNewCmdAssetRateCreate(t *testing.T) {
	// Setup
	mockRepo := new(mockAssetRateRepo)
	f := &factoryAssetRateCreate{
		repoAssetRate: mockRepo,
	}

	// Execute
	cmd := newCmdAssetRateCreate(f)

	// Verify
	assert.Equal(t, "create", cmd.Use)
	assert.Equal(t, "Creates an asset rate.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "from-asset-code", "to-asset-code",
		"rate", "rate-scale", "metadata", "json-file", "help",
	}
	for _, flag := range flags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacCreate(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacCreate(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoAssetRate)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryAssetRateCreateRunE(t *testing.T) {
	tests := []struct {
		name           string
		setupFlags     func(*factoryAssetRateCreate, *cobra.Command)
		setupMocks     func(*mockAssetRateRepo, *factoryAssetRateCreate)
		expectedError  string
		expectedOutput string
	}{
		{
			name: "successfully creates asset rate with flags",
			setupFlags: func(f *factoryAssetRateCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.FromAssetCode = "USD"
				f.ToAssetCode = "EUR"
				f.Rate = "120"
				f.RateScale = "2"
				f.Metadata = "{\"source\":\"test\"}"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("from-asset-code", f.FromAssetCode)
				cmd.Flags().Set("to-asset-code", f.ToAssetCode)
				cmd.Flags().Set("rate", f.Rate)
				cmd.Flags().Set("rate-scale", f.RateScale)
				cmd.Flags().Set("metadata", f.Metadata)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateCreate) {
				expectedInput := mmodel.CreateAssetRateInput{
					FromAssetCode: "USD",
					ToAssetCode:   "EUR",
					Rate:          120,
					RateScale:     2,
					Metadata:      map[string]interface{}{"source": "test"},
				}

				mockRepo.On("Create", "org123", "ledger123", expectedInput).Return(
					&mmodel.AssetRate{ID: "ar123"}, nil,
				)
			},
			expectedOutput: "Asset Rate ar123 created successfully",
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryAssetRateCreate, cmd *cobra.Command) {
				f.OrganizationID = ""
				f.tuiInput = func(message string) (string, error) {
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateCreate) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when ledger ID is missing and input fails",
			setupFlags: func(f *factoryAssetRateCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateCreate) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when repository call fails",
			setupFlags: func(f *factoryAssetRateCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.FromAssetCode = "USD"
				f.ToAssetCode = "EUR"
				f.Rate = "120"
				f.RateScale = "2"
				f.Metadata = "{\"source\":\"test\"}"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("from-asset-code", f.FromAssetCode)
				cmd.Flags().Set("to-asset-code", f.ToAssetCode)
				cmd.Flags().Set("rate", f.Rate)
				cmd.Flags().Set("rate-scale", f.RateScale)
				cmd.Flags().Set("metadata", f.Metadata)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateCreate) {
				expectedInput := mmodel.CreateAssetRateInput{
					FromAssetCode: "USD",
					ToAssetCode:   "EUR",
					Rate:          120,
					RateScale:     2,
					Metadata:      map[string]interface{}{"source": "test"},
				}

				mockRepo.On("Create", "org123", "ledger123", expectedInput).Return(
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
			mockRepo := new(mockAssetRateRepo)

			f := &factory.Factory{
				IOStreams: ios,
			}

			facCreate := &factoryAssetRateCreate{
				factory:       f,
				repoAssetRate: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("from-asset-code", "", "")
			cmd.Flags().String("to-asset-code", "", "")
			cmd.Flags().String("rate", "", "")
			cmd.Flags().String("rate-scale", "", "")
			cmd.Flags().String("metadata", "{}", "")
			cmd.Flags().String("json-file", "", "")

			// Apply test-specific setup
			tt.setupFlags(facCreate, cmd)
			tt.setupMocks(mockRepo, facCreate)

			// Execute
			err := facCreate.runE(cmd, []string{})

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				if tt.expectedOutput != "" {
					// Note: We can't easily verify the output since it's written to iostreams
					// For a more thorough test, we would need to mock iostreams
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestFactoryAssetRateCreateRunEWithJSONFile(t *testing.T) {
	// Create a temporary JSON file
	tempDir := t.TempDir()
	jsonFilePath := filepath.Join(tempDir, "asset_rate.json")
	jsonContent := `{
		"fromAssetCode": "USD",
		"toAssetCode": "EUR",
		"rate": 120,
		"rateScale": 2,
		"metadata": {"source": "file"}
	}`

	err := os.WriteFile(jsonFilePath, []byte(jsonContent), 0644)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		jsonFile       string
		jsonContent    string
		setupMocks     func(*mockAssetRateRepo)
		expectedError  string
		expectedOutput string
	}{
		{
			name:     "successfully creates asset rate from file",
			jsonFile: jsonFilePath,
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				expectedInput := mmodel.CreateAssetRateInput{
					FromAssetCode: "USD",
					ToAssetCode:   "EUR",
					Rate:          120,
					RateScale:     2,
					Metadata:      map[string]interface{}{"source": "file"},
				}

				mockRepo.On("Create", "org123", "ledger123", expectedInput).Return(
					&mmodel.AssetRate{ID: "ar123"}, nil,
				)
			},
			expectedOutput: "Asset Rate ar123 created successfully",
		},
		{
			name:     "fails with invalid JSON file",
			jsonFile: "invalid_path.json",
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "failed to decode the given 'json' file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ios := iostreams.System()
			mockRepo := new(mockAssetRateRepo)

			f := &factory.Factory{
				IOStreams: ios,
			}

			facCreate := &factoryAssetRateCreate{
				factory:       f,
				repoAssetRate: mockRepo,
				flagsCreate: flagsCreate{
					OrganizationID: "org123",
					LedgerID:       "ledger123",
					JSONFile:       tt.jsonFile,
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("json-file", "", "")
			cmd.Flags().Set("json-file", tt.jsonFile)

			// Apply test-specific setup
			tt.setupMocks(mockRepo)

			// Execute
			err := facCreate.runE(cmd, []string{})

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				// Note: We can't easily verify the output since it's written to iostreams
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestFactoryAssetRateCreateSetFlags(t *testing.T) {
	// Setup
	f := &factoryAssetRateCreate{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "from-asset-code", "to-asset-code",
		"rate", "rate-scale", "metadata", "json-file", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestFactoryAssetRateCreateCreateRequestFromFlags(t *testing.T) {
	tests := []struct {
		name          string
		flags         flagsCreate
		tuiInputs     map[string]string
		tuiInputError error
		expectedInput *mmodel.CreateAssetRateInput
		expectedError string
	}{
		{
			name: "successfully creates request from flags",
			flags: flagsCreate{
				FromAssetCode: "USD",
				ToAssetCode:   "EUR",
				Rate:          "120",
				RateScale:     "2",
				Metadata:      "{\"source\":\"test\"}",
			},
			expectedInput: &mmodel.CreateAssetRateInput{
				FromAssetCode: "USD",
				ToAssetCode:   "EUR",
				Rate:          120,
				RateScale:     2,
				Metadata:      map[string]interface{}{"source": "test"},
			},
		},
		{
			name: "successfully creates request with TUI inputs",
			flags: flagsCreate{
				Metadata: "{\"source\":\"test\"}",
			},
			tuiInputs: map[string]string{
				"from-asset-code":                       "USD",
				"to-asset-code":                         "EUR",
				"Enter the rate":                        "120",
				"Enter the rate scale (decimal places)": "2",
			},
			expectedInput: &mmodel.CreateAssetRateInput{
				FromAssetCode: "USD",
				ToAssetCode:   "EUR",
				Rate:          120,
				RateScale:     2,
				Metadata:      map[string]interface{}{"source": "test"},
			},
		},
		{
			name: "fails with invalid rate",
			flags: flagsCreate{
				FromAssetCode: "USD",
				ToAssetCode:   "EUR",
				Rate:          "invalid",
				RateScale:     "2",
				Metadata:      "{\"source\":\"test\"}",
			},
			expectedError: "Error parsing rate",
		},
		{
			name: "fails with invalid rate scale",
			flags: flagsCreate{
				FromAssetCode: "USD",
				ToAssetCode:   "EUR",
				Rate:          "120",
				RateScale:     "invalid",
				Metadata:      "{\"source\":\"test\"}",
			},
			expectedError: "Error parsing rate scale",
		},
		{
			name: "fails with invalid metadata",
			flags: flagsCreate{
				FromAssetCode: "USD",
				ToAssetCode:   "EUR",
				Rate:          "120",
				RateScale:     "2",
				Metadata:      "invalid json",
			},
			expectedError: "Error parsing metadata",
		},
		{
			name: "fails when TUI input fails",
			flags: flagsCreate{
				FromAssetCode: "USD",
				Metadata:      "{\"source\":\"test\"}",
			},
			tuiInputError: errors.New("input error"),
			expectedError: "input error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			f := &factoryAssetRateCreate{
				flagsCreate: tt.flags,
				tuiInput: func(message string) (string, error) {
					if tt.tuiInputError != nil {
						return "", tt.tuiInputError
					}

					for prompt, response := range tt.tuiInputs {
						if message == prompt {
							return response, nil
						}
					}

					return "", errors.New("unexpected prompt: " + message)
				},
			}

			assetRate := &mmodel.CreateAssetRateInput{}

			// Execute
			err := f.createRequestFromFlags(assetRate)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedInput.FromAssetCode, assetRate.FromAssetCode)
				assert.Equal(t, tt.expectedInput.ToAssetCode, assetRate.ToAssetCode)
				assert.Equal(t, tt.expectedInput.Rate, assetRate.Rate)
				assert.Equal(t, tt.expectedInput.RateScale, assetRate.RateScale)
				assert.Equal(t, tt.expectedInput.Metadata, assetRate.Metadata)
			}
		})
	}
}
