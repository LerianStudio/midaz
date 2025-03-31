package login

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/setting"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"
)

// setupTestEnv creates a temporary directory for test settings
func setupTestEnv(t *testing.T) (string, func()) {
	// Create a temporary directory for the test
	tempDir, err := ioutil.TempDir("", "mdz-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create the config directory
	configDir := filepath.Join(tempDir, ".mdz")
	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Set the HOME environment variable to the temp directory
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)

	// Return the temp directory and a cleanup function
	cleanup := func() {
		os.Setenv("HOME", origHome)
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup
}

// Mock for tui.Select
type mockTuiSelect struct {
	mock.Mock
}

func (m *mockTuiSelect) Select(message string, options []string) (string, error) {
	args := m.Called(message, options)
	return args.String(0), args.Error(1)
}

func TestRunEWithCredentials(t *testing.T) {
	tests := []struct {
		name           string
		username       string
		password       string
		expectError    bool
		mockAuthSetup  func(mockAuth *repository.MockAuth)
		expectedOutput string
	}{
		{
			name:        "Successful login",
			username:    "testuser",
			password:    "testpass",
			expectError: false,
			mockAuthSetup: func(mockAuth *repository.MockAuth) {
				mockAuth.
					EXPECT().
					AuthenticateWithCredentials("testuser", "testpass").
					Return(&model.TokenResponse{AccessToken: "mock-token"}, nil)
			},
			expectedOutput: "successfully logged in",
		},
		{
			name:        "Invalid credentials",
			username:    "invaliduser",
			password:    "invalidpass",
			expectError: true,
			mockAuthSetup: func(mockAuth *repository.MockAuth) {
				mockAuth.
					EXPECT().
					AuthenticateWithCredentials("invaliduser", "invalidpass").
					Return(nil, errors.New("invalid credentials"))
			},
			expectedOutput: "",
		},
		{
			name:        "Empty username",
			username:    "",
			password:    "somepass",
			expectError: true,
			mockAuthSetup: func(mockAuth *repository.MockAuth) {
				// No call expected since validation should fail before calling auth
			},
			expectedOutput: "",
		},
		{
			name:        "Empty password",
			username:    "someuser",
			password:    "",
			expectError: true,
			mockAuthSetup: func(mockAuth *repository.MockAuth) {
				// No call expected since validation should fail before calling auth
			},
			expectedOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			_, cleanup := setupTestEnv(t)
			defer cleanup()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAuth := repository.NewMockAuth(ctrl)

			if tt.mockAuthSetup != nil {
				tt.mockAuthSetup(mockAuth)
			}

			outBuf := &bytes.Buffer{}
			errBuf := &bytes.Buffer{}

			f := &factory.Factory{
				IOStreams: &iostreams.IOStreams{
					Out: outBuf,
					Err: errBuf,
				},
				Env: environment.New(),
			}

			l := &factoryLogin{
				factory:   f,
				username:  tt.username,
				password:  tt.password,
				auth:      mockAuth,
				tuiSelect: nil,
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("username", "", "")
			cmd.Flags().String("password", "", "")
			cmd.Flags().Set("username", tt.username)
			cmd.Flags().Set("password", tt.password)

			cmd.Flags().Lookup("username").Changed = true
			cmd.Flags().Lookup("password").Changed = true

			err := l.runE(cmd, []string{})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, outBuf.String(), tt.expectedOutput)

				// Verify that the token was saved to the setting file
				s, err := setting.Read()
				assert.NoError(t, err)
				assert.Equal(t, "mock-token", s.Token)
			}
		})
	}
}

func TestNewCmdLogin(t *testing.T) {
	type args struct {
		f *factory.Factory
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "success",
			args: args{
				f: factory.NewFactory(environment.New()),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCmdLogin(tt.args.f)
			assert.NotNil(t, cmd)
			assert.Equal(t, "login", cmd.Use)
			assert.Equal(t, "Authenticate with Midaz CLI", cmd.Short)
		})
	}
}

// TestValidateCredentials tests the validateCredentials function
func TestValidateCredentials(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		password    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid credentials",
			username:    "testuser",
			password:    "testpass",
			expectError: false,
		},
		{
			name:        "Empty username",
			username:    "",
			password:    "testpass",
			expectError: true,
			errorMsg:    "username must not be empty",
		},
		{
			name:        "Empty password",
			username:    "testuser",
			password:    "",
			expectError: true,
			errorMsg:    "password must not be empty",
		},
		{
			name:        "Both empty",
			username:    "",
			password:    "",
			expectError: true,
			errorMsg:    "username must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCredentials(tt.username, tt.password)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
