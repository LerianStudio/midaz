package configure

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/setting"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryConfigure struct {
	factory  *factory.Factory
	tuiInput func(message string) (string, error)
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
	sett, err := setting.Read()
	if err != nil {
		return err
	}

	if !cmd.Flags().Changed("client-id") {
		clientID, err := tui.Input("Enter your client-id")
		if err != nil {
			return err
		}

		f.ClientID = clientID
	}

	if len(f.ClientID) > 0 {
		sett.ClientID = f.ClientID
	}

	if !cmd.Flags().Changed("client-secret") {
		clientSecret, err := tui.Input("Enter your client-secret")
		if err != nil {
			return err
		}

		f.ClientSecret = clientSecret
	}

	if len(f.ClientSecret) > 0 {
		sett.ClientSecret = f.ClientSecret
	}

	if !cmd.Flags().Changed("url-api-auth") {
		urlAPIAuth, err := tui.Input("Enter your url-api-auth")
		if err != nil {
			return err
		}

		f.URLAPIAuth = urlAPIAuth
	}

	if len(f.URLAPIAuth) > 0 {
		sett.URLAPIAuth = f.URLAPIAuth
	}

	if !cmd.Flags().Changed("url-api-ledger") {
		urlAPILedger, err := tui.Input("Enter your url-api-ledger")
		if err != nil {
			return err
		}

		f.URLAPILedger = urlAPILedger
	}

	if len(f.URLAPILedger) > 0 {
		sett.URLAPILedger = f.URLAPILedger
	}

	err = setting.Save(*sett)
	if err != nil {
		return err
	}

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
	}
}

func NewCmdConfigure(f *factoryConfigure) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Defines the service URL and authentication credentials for the Ledger environment.",
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
