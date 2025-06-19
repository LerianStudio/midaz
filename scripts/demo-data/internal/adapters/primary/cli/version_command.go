package cli

import (
	"fmt"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Build information (set during build)
var (
	version   = "dev"
	commit    = "unknown"
	date      = "unknown"
	goVersion = runtime.Version()
)

// newVersionCommand creates the version command
func (a *CLIAdapter) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		Long:  "Display detailed version information including build details and Go version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runVersionCommand(cmd, args)
		},
	}
}

// runVersionCommand executes the version command
func (a *CLIAdapter) runVersionCommand(cmd *cobra.Command, args []string) error {
	cyan := color.New(color.FgCyan, color.Bold)
	green := color.New(color.FgGreen)
	blue := color.New(color.FgBlue)

	// Header
	cyan.Println("📊 Demo Data Generator")
	fmt.Println()

	// Version information
	fmt.Printf("Version:     %s\n", green.Sprint(version))
	fmt.Printf("Commit:      %s\n", green.Sprint(commit))
	fmt.Printf("Built:       %s\n", green.Sprint(date))
	fmt.Printf("Go version:  %s\n", green.Sprint(goVersion))
	fmt.Printf("Platform:    %s/%s\n", green.Sprint(runtime.GOOS), green.Sprint(runtime.GOARCH))

	// Additional information
	fmt.Println()
	blue.Println("🏗️  Architecture: Hexagonal (Ports & Adapters)")
	blue.Println("🔧 Configuration: Viper + Environment Variables")
	blue.Println("⚡ CLI Framework: Cobra")

	fmt.Println()
	fmt.Printf("For more information, visit: %s\n",
		color.New(color.FgBlue, color.Underline).Sprint("https://github.com/lerianstudio/midaz"))

	return nil
}
