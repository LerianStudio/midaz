package login

import (
	"bytes"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestRunE(t *testing.T) {
	// Define test cases
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
			expectedOutput: "Successfully logged in",
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
					Return(nil, errors.New("invalid credentials")) // Return an error, not a string
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
				f: factory.NewFactory(&environment.Env{}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NewCmdLogin(tt.args.f)
		})
	}
}
