package pkg

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/segmentio/ksuid"
	"github.com/spf13/cobra"
)

const (
	stackFlag        = "stack"
	organizationFlag = "organization"
	outputFlag       = "output"
)

var (

	// ErrOrganizationNotSpecified indicates that no organization was specified when one was required.
	ErrOrganizationNotSpecified = errors.New("organization not specified")

	// ErrMultipleOrganizationsFound indicates that more than one organization was found when only one was expected, and no specific organization was specified.
	ErrMultipleOrganizationsFound = errors.New("found more than one organization and no organization specified")
)

// GetSelectedOrganization retrieves the selected organization from the command.
func GetSelectedOrganization(cmd *cobra.Command) string {
	return GetString(cmd, organizationFlag)
}

// RetrieveOrganizationIDFromFlagOrProfile retrieves the organization ID from the command flag or profile.
func RetrieveOrganizationIDFromFlagOrProfile(cmd *cobra.Command, cfg *Config) (string, error) {
	if id := GetSelectedOrganization(cmd); id != "" {
		return id, nil
	}

	if defaultOrganization := GetCurrentProfile(cmd, cfg).GetDefaultOrganization(); defaultOrganization != "" {
		return defaultOrganization, nil
	}

	return "", ErrOrganizationNotSpecified
}

// GetSelectedStackID retrieves the selected stack ID from the command.
func GetSelectedStackID(cmd *cobra.Command) string {
	return GetString(cmd, stackFlag)
}

// CommandOption is an interface for options that can be applied to a cobra.Command.
type CommandOption interface {
	apply(cmd *cobra.Command)
}

// CommandOptionFn is a function that applies a CommandOption to a cobra.Command.
type CommandOptionFn func(cmd *cobra.Command)

func (fn CommandOptionFn) apply(cmd *cobra.Command) {
	fn(cmd)
}

// WithPersistentStringFlag is a helper function that returns a CommandOptionFn that applies a persistent string flag to a cobra.Command.
func WithPersistentStringFlag(name, defaultValue, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.PersistentFlags().String(name, defaultValue, help)
	}
}

// WithStringFlag is a helper function that returns a CommandOptionFn that applies a string flag to a cobra.Command.
func WithStringFlag(name, defaultValue, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Flags().String(name, defaultValue, help)
	}
}

// WithPersistentStringPFlag is a helper function that returns a CommandOptionFn that applies a persistent string flag to a cobra.Command.
func WithPersistentStringPFlag(name, short, defaultValue, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.PersistentFlags().StringP(name, short, defaultValue, help)
	}
}

// WithBoolFlag is a helper function that returns a CommandOptionFn that applies a boolean flag to a cobra.Command.
func WithBoolFlag(name string, defaultValue bool, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Flags().Bool(name, defaultValue, help)
	}
}

// WithAliases is a helper function that returns a CommandOptionFn that applies aliases to a cobra.Command.
func WithAliases(aliases ...string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Aliases = aliases
	}
}

// WithPersistentBoolPFlag is a helper function that returns a CommandOptionFn that applies a persistent boolean flag to a cobra.Command.
func WithPersistentBoolPFlag(name, short string, defaultValue bool, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.PersistentFlags().BoolP(name, short, defaultValue, help)
	}
}

// WithPersistentBoolFlag is a helper function that returns a CommandOptionFn that applies a persistent boolean flag to a cobra.Command.
func WithPersistentBoolFlag(name string, defaultValue bool, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.PersistentFlags().Bool(name, defaultValue, help)
	}
}

// WithIntFlag is a helper function that returns a CommandOptionFn that applies an integer flag to a cobra.Command.
func WithIntFlag(name string, defaultValue int, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Flags().Int(name, defaultValue, help)
	}
}

// WithStringSliceFlag is a helper function that returns a CommandOptionFn that applies a string slice flag to a cobra.Command.
func WithStringSliceFlag(name string, defaultValue []string, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Flags().StringSlice(name, defaultValue, help)
	}
}

// WithStringArrayFlag is a helper function that returns a CommandOptionFn that applies a string array flag to a cobra.Command.
func WithStringArrayFlag(name string, defaultValue []string, help string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Flags().StringArray(name, defaultValue, help)
	}
}

// WithHiddenFlag is a helper function that returns a CommandOptionFn that applies a hidden flag to a cobra.Command.
func WithHiddenFlag(name string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		_ = cmd.Flags().MarkHidden(name)
	}
}

// WithHidden is a helper function that returns a CommandOptionFn that applies a hidden flag to a cobra.Command.
func WithHidden() CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Hidden = true
	}
}

// WithRunE is a helper function that returns a CommandOptionFn that applies a run function to a cobra.Command.
func WithRunE(fn func(cmd *cobra.Command, args []string) error) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.RunE = fn
	}
}

// WithPersistentPreRunE is a helper function that returns a CommandOptionFn that applies a persistent pre-run function to a cobra.Command.
func WithPersistentPreRunE(fn func(cmd *cobra.Command, args []string) error) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.PersistentPreRunE = fn
	}
}

// WithPreRunE is a helper function that returns a CommandOptionFn that applies a pre-run function to a cobra.Command.
func WithPreRunE(fn func(cmd *cobra.Command, args []string) error) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.PreRunE = fn
	}
}

// WithDeprecatedFlag is a helper function that returns a CommandOptionFn that applies a deprecated flag to a cobra.Command.
func WithDeprecatedFlag(name, message string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		_ = cmd.Flags().MarkDeprecated(name, message)
	}
}

// WithDeprecated is a helper function that returns a CommandOptionFn that applies a deprecated flag to a cobra.Command.
func WithDeprecated(message string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Deprecated = message
	}
}

// WithChildCommands is a helper function that returns a CommandOptionFn that applies child commands to a cobra.Command.
func WithChildCommands(cmds ...*cobra.Command) CommandOptionFn {
	return func(cmd *cobra.Command) {
		for _, child := range cmds {
			cmd.AddCommand(child)
		}
	}
}

// WithShortDescription is a helper function that returns a CommandOptionFn that applies a short description to a cobra.Command.
func WithShortDescription(v string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Short = v
	}
}

// WithArgs is a helper function that returns a CommandOptionFn that applies positional arguments to a cobra.Command.
func WithArgs(p cobra.PositionalArgs) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Args = p
	}
}

// WithValidArgs is a helper function that returns a CommandOptionFn that applies valid arguments to a cobra.Command.
func WithValidArgs(validArgs ...string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.ValidArgs = validArgs
	}
}

// WithValidArgsFunction returns a CommandOptionFn that sets a custom validation function for command arguments.
func WithValidArgsFunction(fn func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.ValidArgsFunction = fn
	}
}

// WithDescription is a helper function that returns a CommandOptionFn that applies a description to a cobra.Command.
func WithDescription(v string) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.Long = v
	}
}

// WithSilenceUsage is a helper function that returns a CommandOptionFn that applies silence usage to a cobra.Command.
func WithSilenceUsage() CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.SilenceUsage = true
	}
}

// WithSilenceError is a helper function that returns a CommandOptionFn that applies silence error to a cobra.Command.
func WithSilenceError() CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.SilenceErrors = true
	}
}

// NewStackCommand is a helper function that returns a new stack command with the given options.
func NewStackCommand(use string, opts ...CommandOption) *cobra.Command {
	cmd := NewMembershipCommand(use,
		append(opts,
			WithPersistentStringFlag(stackFlag, "", "Specific stack (not required if only one stack is present)"),
		)...,
	)

	_ = cmd.RegisterFlagCompletionFunc("stack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cfg, err := GetConfig(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		profile := GetCurrentProfile(cmd, cfg)

		claims, err := profile.getUserInfo()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		selectedOrganization := GetSelectedOrganization(cmd)
		if selectedOrganization == "" {
			selectedOrganization = profile.defaultOrganization
		}

		ret := make([]string, 0)

		for _, org := range claims.Org {
			if selectedOrganization != "" && selectedOrganization != org.ID {
				continue
			}

			for _, stack := range org.Stacks {
				ret = append(ret, fmt.Sprintf("%s\t%s", stack.ID, stack.DisplayName))
			}
		}

		return ret, cobra.ShellCompDirectiveDefault
	})

	return cmd
}

// WithController wraps a controller's Run method as a cobra command's RunE function.
func WithController[T any](c Controller[T]) CommandOptionFn {
	return func(cmd *cobra.Command) {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			renderable, err := c.Run(cmd, args)
			if err != nil {
				return err
			}

			err = WithRender(cmd, args, c, renderable)
			if err != nil {
				return err
			}

			return nil
		}
	}
}

// WithRender handles the rendering of the output based on the output flag.
func WithRender[T any](cmd *cobra.Command, args []string, controller Controller[T], renderable Renderable) error {
	output := GetString(cmd, OutputFlag)

	switch output {
	case "json":
		export := ExportedData{
			Data: controller.GetStore(),
		}

		out, err := json.Marshal(export)
		if err != nil {
			return err
		}

		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(out); err != nil {
			return err
		}
	default:
		return renderable.Render(cmd, args)
	}

	return nil
}

// NewMembershipCommand is a helper function that returns a new membership command with the given options.
func NewMembershipCommand(use string, opts ...CommandOption) *cobra.Command {
	cmd := NewCommand(use,
		append(opts,
			WithPersistentStringFlag(organizationFlag, "", "Selected organization (not required if only one organization is present)"),
		)...,
	)

	_ = cmd.RegisterFlagCompletionFunc("organization", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cfg, err := GetConfig(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		profile := GetCurrentProfile(cmd, cfg)

		claims, err := profile.getUserInfo()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		ret := make([]string, 0)
		for _, org := range claims.Org {
			ret = append(ret, fmt.Sprintf("%s\t%s", org.ID, org.DisplayName))
		}

		return ret, cobra.ShellCompDirectiveDefault
	})

	return cmd
}

// NewCommand creates a new cobra command with the specified use string and options.
//
// Parameters:
// - use: a string representing the use of the command.
// - opts: variadic CommandOption options.
// Return type:
// - *cobra.Command: a pointer to the created cobra command.
func NewCommand(use string, opts ...CommandOption) *cobra.Command {
	cmd := &cobra.Command{
		Use: use,
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if GetBool(cmd, TelemetryFlag) {
				cfg, err := GetConfig(cmd)
				if err != nil {
					return
				}

				if cfg.GetUniqueID() == "" {
					uniqueID := ksuid.New().String()
					cfg.SetUniqueID(uniqueID)
					err = cfg.Persist()
					if err != nil {
						return
					}
				}
			}
		},
	}
	for _, opt := range opts {
		opt.apply(cmd)
	}

	return cmd
}
