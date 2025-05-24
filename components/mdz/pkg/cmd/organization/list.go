package organization

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

type factoryOrganizationList struct {
	factory          *factory.Factory
	repoOrganization repository.Organization
	Limit            int
	Page             int
	SortOrder        string
	StartDate        string
	EndDate          string
	JSON             bool
}

func (f *factoryOrganizationList) runE(cmd *cobra.Command, _ []string) error {
	if len(f.StartDate) > 0 {
		if err := utils.ValidateDate(f.StartDate); err != nil {
			return err
		}
	}

	if len(f.EndDate) > 0 {
		if err := utils.ValidateDate(f.EndDate); err != nil {
			return err
		}
	}

	orgs, err := f.repoOrganization.Get(f.Limit, f.Page, f.SortOrder, f.StartDate, f.EndDate)
	if err != nil {
		return err
	}

	if f.JSON {
		b, err := json.Marshal(orgs)
		if err != nil {
			return err
		}

		output.Printf(f.factory.IOStreams.Out, string(b))

		return nil
	}

	tbl := table.New(
		"ID",
		"PARENT_ORGANIZATION_ID",
		"LEGALNAME",
		"DOING_BUSINESS_AS",
		"LEGAL_DOCUMENT",
		"ADDRESS_COUNTRY",
		"STATUS_CODE",
		"CREATED_AT",
		"UPDATED_AT",
	)

	if !f.factory.NoColor {
		headerFmt := color.New(color.FgYellow).SprintfFunc()
		columnFmt := color.New(color.FgYellow).SprintfFunc()
		tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)
	}

	tbl.WithWriter(f.factory.IOStreams.Out)

	for _, i := range orgs.Items {
		tbl.AddRow(
			i.ID,
			utils.SafeString(i.ParentOrganizationID),
			i.LegalName,
			utils.SafeString(i.DoingBusinessAs),
			i.LegalDocument,
			i.Address.Country,
			i.Status.Code,
			i.CreatedAt,
			i.UpdatedAt,
		)
	}

	tbl.Print()

	// Add interactive selection if in REPL mode
	if utils.IsInREPL() && !f.JSON {
		return f.offerInteractiveSelection(orgs.Items)
	}

	return nil
}

// offerInteractiveSelection allows users to select an organization to set as context
func (f *factoryOrganizationList) offerInteractiveSelection(orgs []mmodel.Organization) error {
	if len(orgs) == 0 {
		return nil
	}

	// Convert to interactive selectors
	items := make([]utils.InteractiveSelector, len(orgs))

	for i, org := range orgs {
		description := fmt.Sprintf("%s | %s", org.LegalDocument, org.Address.Country)
		items[i] = utils.InteractiveSelector{
			ID:          org.ID,
			Name:        org.LegalName,
			Description: description,
			Type:        "organization",
		}
	}

	// Offer selection
	selected, err := utils.OfferInteractiveSelection(f.factory, items, "organization")
	if err != nil {
		return err
	}

	// If user selected something, set context and announce the change
	if selected != nil {
		err := utils.SetREPLContext(context.TODO(), "organization", selected.ID, selected.Name)
		if err != nil {
			return err
		}

		fmt.Fprintf(f.factory.IOStreams.Out, "\n🎯 Organization context set to: %s\n", selected.Name)
		fmt.Fprintf(f.factory.IOStreams.Out, "💡 You can now run 'ledger list' to see ledgers in this organization.\n")
	}

	return nil
}

func (f *factoryOrganizationList) setFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().IntVar(&f.Limit, "limit", 10,
		"Specifies the number of organizations to retrieve per page")
	cmd.Flags().IntVar(&f.Page, "page", 1,
		"Specifies the page number for paginated results")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "",
		"Specifies the sort order for results (e.g., 'asc' for ascending, 'desc' for descending)")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "",
		"Specifies the start date for filtering results (format: YYYY-MM-DD)")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "",
		"Specifies the end date for filtering results (format: YYYY-MM-DD)")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryOrganizationList {
	return &factoryOrganizationList{
		factory:          f,
		repoOrganization: rest.NewOrganization(f),
	}
}

func newCmdOrganizationList(f *factoryOrganizationList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List organizations in Midaz",
		Long: "The organization list command allows you to retrieve a list of organizations " +
			"in Midaz. You can paginate the results using the limit and page options, " +
			"and optionally retrieve a specific number of organizations per page.",
		Example: utils.Format(
			"$ mdz organization list",
			"$ mdz organization list -h",
			"$ mdz organization list --limit 10",
			"$ mdz organization list --page 3",
			"$ mdz organization list --limit 10 --page 2",
			"$ mdz organization list --sort-order desc --start-date 2024-12-10 --end-date 2024-12-20",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
