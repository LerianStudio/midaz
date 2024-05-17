package pkg

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"
)

const (
	// MembershipURIFlag specifies the URI for membership
	MembershipURIFlag = "membership-uri"
	// FileFlag specifies the configuration file
	FileFlag = "config"
	// ProfileFlag specifies the profile to use
	ProfileFlag = "profile"
	// OutputFlag specifies the output format
	OutputFlag = "output"
	// DebugFlag specifies whether to run the command in debug mode
	DebugFlag = "debug"
	// TelemetryFlag specifies whether to enable telemetry
	TelemetryFlag = "telemetry"
)

// GetBool retrieves the boolean value of the specified flag from the command.
func GetBool(cmd *cobra.Command, flagName string) bool {
	v, err := cmd.Flags().GetBool(flagName)
	if err != nil {
		fromEnv := strings.ToLower(os.Getenv(strcase.ToScreamingSnake(flagName)))
		return fromEnv == "true" || fromEnv == "1"
	}

	return v
}

// GetString retrieves the string value of the specified flag from the command.
func GetString(cmd *cobra.Command, flagName string) string {
	v, err := cmd.Flags().GetString(flagName)
	if err != nil || v == "" {
		return os.Getenv(strcase.ToScreamingSnake(flagName))
	}

	return v
}

// GetStringSlice retrieves the string slice value of the specified flag from the command.
func GetStringSlice(cmd *cobra.Command, flagName string) []string {
	v, err := cmd.Flags().GetStringSlice(flagName)
	if err != nil || len(v) == 0 {
		envVar := os.Getenv(strcase.ToScreamingSnake(flagName))
		if envVar == "" {
			return []string{}
		}

		return strings.Split(envVar, " ")
	}

	return v
}

// GetInt retrieves the integer value of the specified flag from the command.
func GetInt(cmd *cobra.Command, flagName string) int {
	v, err := cmd.Flags().GetInt(flagName)
	if err != nil {
		v := os.Getenv(strcase.ToScreamingSnake(flagName))
		if v != "" {
			v, err := strconv.Atoi(v)
			if err != nil {
				return 0
			}

			return v
		}

		return 0
	}

	return v
}

// GetDateTime retrieves the time value of the specified flag from the command.
func GetDateTime(cmd *cobra.Command, flagName string) (*time.Time, error) {
	v, err := cmd.Flags().GetString(flagName)
	if err != nil || v == "" {
		v = os.Getenv(strcase.ToScreamingSnake(flagName))
	}

	if v == "" {
		return nil, nil
	}

	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

// Ptr returns a pointer to the given value.
func Ptr[T any](t T) *T {
	return &t
}
