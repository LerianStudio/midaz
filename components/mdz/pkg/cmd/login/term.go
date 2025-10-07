// Package login provides the CLI login command for authentication.
// This file contains terminal-based username/password login functionality.
package login

import (
	"github.com/LerianStudio/midaz/v3/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/tui"
)

// terminalLogin implements username/password authentication via terminal prompts.
//
// This method:
// 1. Prompts for username if not provided
// 2. Prompts for password if not provided (masked input)
// 3. Authenticates with credentials via REST API
// 4. Stores access token
//
// Returns:
//   - error: Error if authentication fails or user cancels input
func (l *factoryLogin) terminalLogin() error {
	var err error

	if len(l.username) == 0 {
		l.username, err = tui.Input("Enter your username")
		if err != nil {
			return err
		}
	}

	if len(l.password) == 0 {
		l.password, err = tui.Password("Enter your password")
		if err != nil {
			return err
		}
	}

	r := rest.Auth{Factory: l.factory}

	t, err := r.AuthenticateWithCredentials(l.username, l.password)
	if err != nil {
		return err
	}

	l.token = t.AccessToken

	return nil
}
