package login

import (
	"errors"
	"strings"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/setting"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryLogin struct {
	factory   *factory.Factory
	username  string
	password  string
	token     string
	browser   browser
	auth      repository.Auth
	tuiSelect func(message string, options []string) (string, error)
}

func validateCredentials(username, password string) error {
	if len(username) == 0 {
		return errors.New("username must not be empty")
	}

	if len(password) == 0 {
		return errors.New("password must not be empty")
	}

	return nil
}

func (l *factoryLogin) runE(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Changed("username") && cmd.Flags().Changed("password") {
		if err := validateCredentials(l.username, l.password); err != nil {
			return err
		}

		t, err := l.auth.AuthenticateWithCredentials(l.username, l.password)

		if err != nil {
			return err
		}

		l.token = t.AccessToken
	} else {
		option, err := tui.Select(
			"Choose a login method:",
			[]string{"Log in via browser", "Log in via terminal"},
		)

		if err != nil {
			return err
		}

		err = l.execMethodLogin(option)
		if err != nil {
			return err
		}
	}

	sett, err := setting.Read()

	if err != nil {
		return err
	}

	sett.Token = l.token

	if err := setting.Save(*sett); err != nil {
		return err
	}

	output.Printf(l.factory.IOStreams.Out, "successfully logged in")

	return nil
}

func (l *factoryLogin) execMethodLogin(answer string) error {
	switch {
	case strings.Contains(answer, "browser"):
		l.browserLogin()

		if l.browser.Err != nil {
			return l.browser.Err
		}

		return nil
	case strings.Contains(answer, "terminal"):
		err := l.terminalLogin()

		if err := validateCredentials(l.username, l.password); err != nil {
			return err
		}

		if err != nil {
			return err
		}

		return nil
	}

	return errors.New("invalid login method")
}

// \1 performs an operation
func NewCmdLogin(f *factory.Factory) *cobra.Command {
	fVersion := factoryLogin{
		factory:   f,
		auth:      rest.NewAuth(f),
		tuiSelect: tui.Select,
	}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Midaz CLI",
		Long:  "Authenticate with the Midaz CLI using your credentials to gain access to the platform's features.",
		Example: utils.Format(
			"$ mdz login",
			"$ mdz login --username email@examle.com --password Pass@123",
			"$ mdz login -h",
		),
		RunE: fVersion.runE,
	}

	cmd.Flags().StringVar(&fVersion.username, "username", "", "Your username")
	cmd.Flags().StringVar(&fVersion.password, "password", "", "Your password")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the login command")

	return cmd
}
