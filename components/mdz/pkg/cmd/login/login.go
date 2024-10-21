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
	"github.com/fatih/color"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

type factoryLogin struct {
	factory  *factory.Factory
	username string
	password string
	token    string
	browser  browser
	auth     repository.Auth
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

		r := rest.Auth{Factory: l.factory}
		_, err := r.AuthenticateWithCredentials(l.username, l.password)

		if err != nil {
			return err
		}

		output.Printf(l.factory.IOStreams.Out, color.New(color.Bold).Sprint("Successfully logged in"))

		return nil
	}

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

	sett := setting.Setting{
		Token: l.token,
	}

	b, err := toml.Marshal(sett)
	if err != nil {
		output.Printf(l.factory.IOStreams.Err, "Error while marshalling toml file "+err.Error())
		return err
	}

	if err := sett.Save(b); err != nil {
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

func NewCmdLogin(f *factory.Factory) *cobra.Command {
	fVersion := factoryLogin{
		factory: f,
		auth:    rest.NewAuth(f),
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
