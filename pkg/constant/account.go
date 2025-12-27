// Package constant provides constants used across the application.
package constant

// DefaultExternalAccountAliasPrefix is the default prefix for external account aliases.
const (
	DefaultExternalAccountAliasPrefix = "@external/"
	ExternalAccountType               = "external"
	AccountAliasAcceptedChars         = `^[a-zA-Z0-9@:_-]+$`
)
