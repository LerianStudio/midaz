package constant

// Account-related constants used for validation and type identification.
const (
	// DefaultExternalAccountAliasPrefix is the reserved prefix for external account aliases.
	DefaultExternalAccountAliasPrefix = "@external/"
	// ExternalAccountType is the string identifier for external account type.
	ExternalAccountType = "external"
	// AccountAliasAcceptedChars is the allowed character set for account aliases (regex).
	AccountAliasAcceptedChars = `^[a-zA-Z0-9@:_-]+$`
)
