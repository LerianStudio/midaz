package root

import (
	"errors"

	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/account"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/asset"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/configure"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/ledger"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/login"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/organization"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/portfolio"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/segment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/version"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/setting"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type factoryRoot struct {
	factory *factory.Factory
}

func (f *factoryRoot) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(version.NewCmdVersion(f.factory))
	cmd.AddCommand(login.NewCmdLogin(f.factory))
	cmd.AddCommand(organization.NewCmdOrganization(f.factory))
	cmd.AddCommand(ledger.NewCmdLedger(f.factory))
	cmd.AddCommand(asset.NewCmdAsset(f.factory))
	cmd.AddCommand(portfolio.NewCmdPortfolio(f.factory))
	cmd.AddCommand(segment.NewCmdSegment(f.factory))
	cmd.AddCommand(account.NewCmdAccount(f.factory))
	cmd.AddCommand(configure.NewCmdConfigure(configure.NewInjectFacConfigure(f.factory)))
}

func (f *factoryRoot) setFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVar(&f.factory.NoColor, "no-color", false, "Changes the output format passing the json value to the flag")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func (f *factoryRoot) persistentPreRunE(cmd *cobra.Command, _ []string) error {
	name := cmd.Name()

	if name == "completion" ||
		name == "bash" ||
		name == "fish" ||
		name == "powershell" ||
		name == "zsh" ||
		name == "version" {
		return nil
	}

	sett, err := setting.Read()

	if err != nil {
		return errors.New("Try the login command first 'mdz login -h' " + err.Error())
	}

	if len(sett.ClientID) > 0 {
		f.factory.Env.ClientID = sett.ClientID
	}

	if len(sett.ClientSecret) > 0 {
		f.factory.Env.ClientSecret = sett.ClientSecret
	}

	if len(sett.URLAPIAuth) > 0 {
		f.factory.Env.URLAPIAuth = sett.URLAPIAuth
	}

	if len(sett.URLAPILedger) > 0 {
		f.factory.Env.URLAPILedger = sett.URLAPILedger
	}

	if len(sett.Token) > 0 {
		f.factory.Token = sett.Token
	}

	return nil
}

// \1 performs an operation
func NewCmdRoot(f *factory.Factory) *cobra.Command {
	fRoot := factoryRoot{
		factory: f,
	}

	cmd := &cobra.Command{
		Use:   "mdz",
		Short: "Midaz CLI is a command-line tool for interacting with the Midaz ledger",
		Long: utils.Format(
			"Midaz is an open-source ledger designed to offer multi-asset and multi-currency",
			"transaction capabilities within a single, natively immutable and fully auditable platform.",
		),
		Version: fRoot.factory.Env.Version,
		Example: utils.Format(
			"$ mdz",
			"$ mdz -h",
			"$ mdz --help",
		),
		PersistentPreRunE: fRoot.persistentPreRunE,
		SilenceErrors:     true, // Silence errors, so the help message won't be shown on flag error
		SilenceUsage:      true, // Silence usage on error
	}

	cmd.SetIn(fRoot.factory.IOStreams.In)
	cmd.SetOut(fRoot.factory.IOStreams.Out)
	cmd.SetErr(fRoot.factory.IOStreams.Err)

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fRoot.help(cmd, args)
	})

	cmd.SetVersionTemplate(color.New(color.Bold).Sprint(fRoot.factory.Env.Version))
	fRoot.setCmds(cmd)
	fRoot.setFlags(cmd)

	return cmd
}
