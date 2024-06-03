package ui

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/LerianStudio/midaz/components/mdz/pkg"
	"github.com/spf13/cobra"
)

// Payload is a struct designed to encapsulate payload data.
type Payload struct {
	StackURL string `json:"stackUrl"`
	Found    bool   `json:"browserFound"`
}

// Controller is a struct designed to encapsulate payload data.
type Controller struct {
	store *Payload
}

var _ pkg.Controller[*Payload] = (*Controller)(nil)

// NewDefaultUIStore is a func that returns a *UiStruct struct
func NewDefaultUIStore() *Payload {
	return &Payload{
		StackURL: "https://console.midaz.cloud",
		Found:    false,
	}
}

// NewUIController is a func that returns a *Controller struct
func NewUIController() *Controller {
	return &Controller{
		store: NewDefaultUIStore(),
	}
}

func openURL(url string) error {
	var (
		cmd  string
		args []string
	)

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}

	args = append(args, url)

	return startCommand(cmd, args...)
}

func startCommand(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)

	return c.Start()
}

// GetStore is a func that returns a *Payload struct
func (c *Controller) GetStore() *Payload {
	return c.store
}

// Run is a fun that executes Open func and return an interface
func (c *Controller) Run(cmd *cobra.Command, args []string) (pkg.Renderable, error) {
	if err := openURL(c.store.StackURL); err != nil {
		c.store.Found = true
	}

	return c, nil
}

// Render is a func that open Url
func (c *Controller) Render(cmd *cobra.Command, args []string) error {
	fmt.Println("Opening url: ", c.store.StackURL)

	return nil
}

// NewCommand is a func that execute some commands
func NewCommand() *cobra.Command {
	return pkg.NewStackCommand("ui",
		pkg.WithShortDescription("Open UI"),
		pkg.WithArgs(cobra.ExactArgs(0)),
		pkg.WithController[*Payload](NewUIController()),
	)
}
