package configure

import (
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/setting"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/tui"
	"github.com/fatih/color"
	"github.com/rodaine/table"

	"github.com/spf13/cobra"
)

type factoryConfigure struct {
	factory  *factory.Factory
	tuiInput func(message string) (string, error)
	read     func() (*setting.Setting, error)
	save     func(sett setting.Setting) error
	flagsConfigure
}

type flagsConfigure struct {
	ClientID     string
	ClientSecret string
	URLAPIAuth   string
	URLAPILedger string
	JSONFile     string
}

func (f *factoryConfigure) runE(cmd *cobra.Command, _ []string) error {
	sett, err := f.read()
	if err != nil {
		return err
	}

	if !cmd.Flags().Changed("client-id") {
		clientID, err := f.tuiInput("Enter your client-id")
		if err != nil {
			return err
		}

		f.ClientID = clientID
	}

	if len(f.ClientID) > 0 {
		sett.ClientID = f.ClientID
	}

	if !cmd.Flags().Changed("client-secret") {
		clientSecret, err := f.tuiInput("Enter your client-secret")
		if err != nil {
			return err
		}

		f.ClientSecret = clientSecret
	}

	if len(f.ClientSecret) > 0 {
		sett.ClientSecret = f.ClientSecret
	}

	if !cmd.Flags().Changed("url-api-auth") {
		urlAPIAuth, err := f.tuiInput("Enter your url-api-auth")
		if err != nil {
			return err
		}

		f.URLAPIAuth = urlAPIAuth
	}

	if len(f.URLAPIAuth) > 0 {
		sett.URLAPIAuth = f.URLAPIAuth
	}

	if !cmd.Flags().Changed("url-api-ledger") {
		urlAPILedger, err := f.tuiInput("Enter your url-api-ledger")
		if err != nil {
			return err
		}

		f.URLAPILedger = urlAPILedger
	}

	if len(f.URLAPILedger) > 0 {
		sett.URLAPILedger = f.URLAPILedger
	}

	err = f.save(*sett)
	if err != nil {
		return err
	}

	tbl := table.New("FIELDS", "VALUES")

	if !f.factory.NoColor {
		headerFmt := color.New(color.FgYellow).SprintfFunc()
		fieldFmt := color.New(color.FgYellow).SprintfFunc()
		tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(fieldFmt)
	}

	tbl.WithWriter(f.factory.IOStreams.Out)

	tbl.AddRow("client-id:", f.ClientID)
	tbl.AddRow("client-secret:", f.ClientSecret)
	tbl.AddRow("url-api-auth:", f.URLAPIAuth)
	tbl.AddRow("url-api-ledger:", f.URLAPILedger)

	tbl.Print()

	return nil
}

func (f *factoryConfigure) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.ClientID, "client-id", "", "Unique client identifier used for authentication.")
	cmd.Flags().StringVar(&f.ClientSecret, "client-secret", "", "Secret key used to validate the client's identity.")
	cmd.Flags().StringVar(&f.URLAPIAuth, "url-api-auth", "", "URL of the authentication service.")
	cmd.Flags().StringVar(&f.URLAPILedger, "url-api-ledger", "", "URL of the service responsible for the ledger.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func NewInjectFacConfigure(f *factory.Factory) *factoryConfigure {
	return &factoryConfigure{
		factory:  f,
		tuiInput: tui.Input,
		read:     setting.Read,
		save:     setting.Save,
	}
}

func NewCmdConfigure(f *factoryConfigure) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Defines the service URLs and credentials for the mdz CLI environment (config: ~/.config/mdz/mdz.toml).",
		Long: utils.Format(
			"The mdz CLI configure command allows you to define the URL of the",
			"service endpoint and the authentication credentials required to",
			"access the Ledger environment. It offers simple and secure",
			"configuration, which can be done interactively or directly via",
			"command line arguments. Ideal for ensuring efficient integration",
			"with the service.",
		),
		Example: utils.Format(
			"$ mdz configure",
			"$ mdz configure -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
