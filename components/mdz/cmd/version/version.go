package version

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// Version variable to set mdz version
var Version = "mdz version mdz1.0.0"

// Store is a struct designed to encapsulate payload data.
type Store struct {
	Version   string `json:"version"`
	BuildDate string `json:"buildDate"`
	Commit    string `json:"commit"`
}

// Controller is a struct that return *VersionStore struct
type Controller struct {
	store *Store
}

var _ pkg.Controller[*Store] = (*Controller)(nil)

// NewDefaultVersionStore return a *VersionStore struct with the version
func NewDefaultVersionStore() *Store {
	return &Store{
		Version: Version,
	}
}

// NewVersionController return a *Controller struct with a new version store
func NewVersionController() *Controller {
	return &Controller{
		store: NewDefaultVersionStore(),
	}
}

// NewCommand is a func that execute some commands and return a *cobra.Command struct
func NewCommand() *cobra.Command {
	return pkg.NewCommand("version",
		pkg.WithShortDescription("Get version"),
		pkg.WithArgs(cobra.ExactArgs(0)),
		pkg.WithController(NewVersionController()),
	)
}

// GetStore is a func that return a *VersionStore struct
func (c *Controller) GetStore() *Store {
	return c.store
}

// Run is a func that return a Renderable interface
func (c *Controller) Run(cmd *cobra.Command, args []string) (pkg.Renderable, error) {
	return c, nil
}

// Render is a func that receive a struct *cobra.Command
func (c *Controller) Render(cmd *cobra.Command, args []string) error {
	tableData := pterm.TableData{}
	tableData = append(tableData, []string{pterm.LightCyan("Version"), c.store.Version})

	return pterm.DefaultTable.
		WithWriter(cmd.OutOrStdout()).
		WithData(tableData).
		Render()
}
