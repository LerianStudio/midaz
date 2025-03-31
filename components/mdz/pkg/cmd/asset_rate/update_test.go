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
)

func TestNewCmdAssetRateUpdate(t *testing.T) {
	// Setup
	mockRepo := new(mockAssetRateRepo)
	f := &factoryAssetRateUpdate{
		repoAssetRate: mockRepo,
	}

	// Execute
	cmd := newCmdAssetRateUpdate(f)

	// Verify
	assert.Equal(t, "update", cmd.Use)
	assert.Equal(t, "Updates an asset rate.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "asset-rate-id", "rate", "rate-scale",
		"status-code", "status-description", "metadata", "json-file", "help",
	}
	for _, flag := range flags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacUpdate(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacUpdate(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoAssetRate)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryAssetRateUpdateRunE(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func(*factoryAssetRateUpdate, *cobra.Command)
		setupMocks    func(*mockAssetRateRepo, *factoryAssetRateUpdate)
		expectedError string
	}{
		{
			name: "successfully updates asset rate with flags",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"
				f.Rate = "125"
				f.RateScale = "2"
				f.StatusCode = "active"
				f.StatusDescription = "Active rate"
				f.Metadata = "{\"source\":\"test\"}"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
				cmd.Flags().Set("rate", f.Rate)
				cmd.Flags().Set("rate-scale", f.RateScale)
				cmd.Flags().Set("status-code", f.StatusCode)
				cmd.Flags().Set("status-description", f.StatusDescription)
				cmd.Flags().Set("metadata", f.Metadata)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				description := "Active rate"
				expectedInput := mmodel.UpdateAssetRateInput{
					Rate:      125,
					RateScale: 2,
					Status: mmodel.Status{
						Code:        "active",
						Description: &description,
					},
					Metadata: map[string]interface{}{"source": "test"},
				}

				mockRepo.On("Update", "org123", "ledger123", "ar123", expectedInput).Return(
					&mmodel.AssetRate{ID: "ar123"}, nil,
				)
			},
		},
		{
			name: "successfully updates asset rate with partial fields",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"
				f.Rate = "125"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
				cmd.Flags().Set("rate", f.Rate)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				expectedInput := mmodel.UpdateAssetRateInput{
					Rate: 125,
				}

				mockRepo.On("Update", "org123", "ledger123", "ar123", expectedInput).Return(
					&mmodel.AssetRate{ID: "ar123"}, nil,
				)
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = ""
				f.tuiInput = func(message string) (string, error) {
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when ledger ID is missing and input fails",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when asset rate ID is missing and input fails",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					} else if message == "Enter your ledger-id" {
						return "ledger123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails with invalid rate",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"
				f.Rate = "invalid"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
				cmd.Flags().Set("rate", f.Rate)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "Error parsing rate",
		},
		{
			name: "fails with invalid rate scale",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"
				f.Rate = "125"
				f.RateScale = "invalid"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
				cmd.Flags().Set("rate", f.Rate)
				cmd.Flags().Set("rate-scale", f.RateScale)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "Error parsing rate scale",
		},
		{
			name: "fails with invalid metadata",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"
				f.Metadata = "invalid json"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
				cmd.Flags().Set("metadata", f.Metadata)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "Error parsing metadata",
		},
		{
			name: "fails when repository call fails",
			setupFlags: func(f *factoryAssetRateUpdate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"
				f.Rate = "125"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
				cmd.Flags().Set("rate", f.Rate)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo, f *factoryAssetRateUpdate) {
				expectedInput := mmodel.UpdateAssetRateInput{
					Rate: 125,
				}

				mockRepo.On("Update", "org123", "ledger123", "ar123", expectedInput).Return(
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

			facUpdate := &factoryAssetRateUpdate{
				factory:       f,
				repoAssetRate: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("asset-rate-id", "", "")
			cmd.Flags().String("rate", "", "")
			cmd.Flags().String("rate-scale", "", "")
			cmd.Flags().String("status-code", "", "")
			cmd.Flags().String("status-description", "", "")
			cmd.Flags().String("metadata", "{}", "")
			cmd.Flags().String("json-file", "", "")

			// Apply test-specific setup
			tt.setupFlags(facUpdate, cmd)
			tt.setupMocks(mockRepo, facUpdate)

			// Execute
			err := facUpdate.runE(cmd, []string{})

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

func TestFactoryAssetRateUpdateRunEWithJSONFile(t *testing.T) {
	// Create a temporary JSON file
	tempDir := t.TempDir()
	jsonFilePath := filepath.Join(tempDir, "asset_rate_update.json")
	jsonContent := `{
		"rate": 125,
		"rateScale": 2,
		"status": {
			"code": "active",
			"description": "Active rate"
		},
		"metadata": {"source": "file"}
	}`

	err := os.WriteFile(jsonFilePath, []byte(jsonContent), 0644)
	assert.NoError(t, err)

	tests := []struct {
		name          string
		jsonFile      string
		setupMocks    func(*mockAssetRateRepo)
		expectedError string
	}{
		{
			name:     "successfully updates asset rate from file",
			jsonFile: jsonFilePath,
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				description := "Active rate"
				expectedInput := mmodel.UpdateAssetRateInput{
					Rate:      125,
					RateScale: 2,
					Status: mmodel.Status{
						Code:        "active",
						Description: &description,
					},
					Metadata: map[string]interface{}{"source": "file"},
				}

				mockRepo.On("Update", "org123", "ledger123", "ar123", expectedInput).Return(
					&mmodel.AssetRate{ID: "ar123"}, nil,
				)
			},
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

			facUpdate := &factoryAssetRateUpdate{
				factory:       f,
				repoAssetRate: mockRepo,
				flagsUpdate: flagsUpdate{
					OrganizationID: "org123",
					LedgerID:       "ledger123",
					AssetRateID:    "ar123",
					JSONFile:       tt.jsonFile,
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("json-file", "", "")
			cmd.Flags().Set("json-file", tt.jsonFile)

			// Apply test-specific setup
			tt.setupMocks(mockRepo)

			// Execute
			err := facUpdate.runE(cmd, []string{})

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

func TestFactoryAssetRateUpdateSetFlags(t *testing.T) {
	// Setup
	f := &factoryAssetRateUpdate{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "asset-rate-id", "rate", "rate-scale",
		"status-code", "status-description", "metadata", "json-file", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestFactoryAssetRateUpdateUpdateRequestFromFlags(t *testing.T) {
	tests := []struct {
		name          string
		flags         flagsUpdate
		expectedInput *mmodel.UpdateAssetRateInput
		expectedError string
	}{
		{
			name: "successfully creates request with all fields",
			flags: flagsUpdate{
				Rate:              "125",
				RateScale:         "2",
				StatusCode:        "active",
				StatusDescription: "Active rate",
				Metadata:          "{\"source\":\"test\"}",
			},
			expectedInput: &mmodel.UpdateAssetRateInput{
				Rate:      125,
				RateScale: 2,
				Status: mmodel.Status{
					Code:        "active",
					Description: func() *string { s := "Active rate"; return &s }(),
				},
				Metadata: map[string]interface{}{"source": "test"},
			},
		},
		{
			name: "successfully creates request with rate only",
			flags: flagsUpdate{
				Rate: "125",
			},
			expectedInput: &mmodel.UpdateAssetRateInput{
				Rate: 125,
			},
		},
		{
			name: "successfully creates request with status only",
			flags: flagsUpdate{
				StatusCode: "active",
			},
			expectedInput: &mmodel.UpdateAssetRateInput{
				Status: mmodel.Status{
					Code: "active",
				},
			},
		},
		{
			name: "successfully creates request with empty metadata",
			flags: flagsUpdate{
				Rate:     "125",
				Metadata: "{}",
			},
			expectedInput: &mmodel.UpdateAssetRateInput{
				Rate: 125,
			},
		},
		{
			name: "fails with invalid rate",
			flags: flagsUpdate{
				Rate: "invalid",
			},
			expectedError: "Error parsing rate",
		},
		{
			name: "fails with invalid rate scale",
			flags: flagsUpdate{
				RateScale: "invalid",
			},
			expectedError: "Error parsing rate scale",
		},
		{
			name: "fails with invalid metadata",
			flags: flagsUpdate{
				Metadata: "invalid json",
			},
			expectedError: "Error parsing metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			f := &factoryAssetRateUpdate{
				flagsUpdate: tt.flags,
			}

			assetRate := &mmodel.UpdateAssetRateInput{}

			// Execute
			err := f.updateRequestFromFlags(assetRate)

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)

				if tt.expectedInput.Rate != 0 {
					assert.Equal(t, tt.expectedInput.Rate, assetRate.Rate)
				}

				if tt.expectedInput.RateScale != 0 {
					assert.Equal(t, tt.expectedInput.RateScale, assetRate.RateScale)
				}

				if tt.expectedInput.Status.Code != "" {
					assert.Equal(t, tt.expectedInput.Status.Code, assetRate.Status.Code)

					if tt.expectedInput.Status.Description != nil {
						assert.NotNil(t, assetRate.Status.Description)
						assert.Equal(t, *tt.expectedInput.Status.Description, *assetRate.Status.Description)
					}
				}

				assert.Equal(t, tt.expectedInput.Metadata, assetRate.Metadata)
			}
		})
	}
}
