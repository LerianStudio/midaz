package organization

import (
	"encoding/json"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"

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
