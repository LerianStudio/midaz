package balance

import (
	"errors"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mpointers"

	"github.com/spf13/cobra"
)

type factoryBalanceUpdate struct {
	factory     *factory.Factory
	repoBalance repository.Balance
	tuiInput    func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID string
	LedgerID       string
	BalanceID      string
	AllowSending   string
	AllowReceiving string
	JSONFile       string
}

func (f *factoryBalanceUpdate) runE(cmd *cobra.Command, _ []string) error {
	balance := mmodel.UpdateBalance{}

	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := f.tuiInput("Enter your organization-id")
		if err != nil {
			return err
		}

		f.OrganizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.LedgerID) < 1 {
		id, err := f.tuiInput("Enter your ledger-id")
		if err != nil {
			return err
		}

		f.LedgerID = id
	}

	if !cmd.Flags().Changed("balance-id") && len(f.BalanceID) < 1 {
		id, err := f.tuiInput("Enter the balance-id")
		if err != nil {
			return err
		}

		f.BalanceID = id
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &balance)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&balance)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoBalance.Update(f.OrganizationID, f.LedgerID, f.BalanceID, balance)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp, "Balance", output.Updated)

	return nil
}

func (f *factoryBalanceUpdate) createRequestFromFlags(balance *mmodel.UpdateBalance) error {
	if len(f.AllowSending) > 0 {
		allowSending, err := utils.ParseBool(f.AllowSending)
		if err != nil {
			return err
		}

		balance.AllowSending = mpointers.Bool(allowSending)
	}

	if len(f.AllowReceiving) > 0 {
		allowReceiving, err := utils.ParseBool(f.AllowReceiving)
		if err != nil {
			return err
		}

		balance.AllowReceiving = mpointers.Bool(allowReceiving)
	}

	return nil
}

func (f *factoryBalanceUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.BalanceID, "balance-id", "", "Specify the balance ID.")
	cmd.Flags().StringVar(&f.AllowSending, "allow-sending", "", "Allow sending funds from this balance (true/false).")
	cmd.Flags().StringVar(&f.AllowReceiving, "allow-receiving", "", "Allow receiving funds to this balance (true/false).")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing balance attributes, or '-' for stdin.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryBalanceUpdate {
	return &factoryBalanceUpdate{
		factory:     f,
		repoBalance: rest.NewBalance(f),
		tuiInput:    tui.Input,
	}
}

func newCmdBalanceUpdate(f *factoryBalanceUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates a balance.",
		Long: utils.Format(
			"Updates a specific balance's attributes, such as allowing sending or receiving funds.",
			"Returns the updated balance information.",
		),
		Example: utils.Format(
			"$ mdz balance update",
			"$ mdz balance update -h",
			"$ mdz balance update --organization-id 123 --ledger-id 456 --balance-id 789 --allow-sending true",
			"$ mdz balance update --json-file balance-updates.json",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
