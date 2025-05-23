package repl

import (
	"fmt"
	"strings"
)

// Context holds the current REPL context for chained operations
type Context struct {
	OrganizationID   string
	OrganizationName string
	LedgerID         string
	LedgerName       string
	PortfolioID      string
	PortfolioName    string
	AccountID        string
	AccountName      string
}

// NewContext creates a new REPL context
func NewContext() *Context {
	return &Context{}
}

// Clear clears all context
func (c *Context) Clear() {
	c.OrganizationID = ""
	c.OrganizationName = ""
	c.LedgerID = ""
	c.LedgerName = ""
	c.PortfolioID = ""
	c.PortfolioName = ""
	c.AccountID = ""
	c.AccountName = ""
}

// ClearLedger clears ledger and dependent context
func (c *Context) ClearLedger() {
	c.LedgerID = ""
	c.LedgerName = ""
	c.ClearPortfolio()
	c.ClearAccount()
}

// ClearPortfolio clears portfolio and dependent context
func (c *Context) ClearPortfolio() {
	c.PortfolioID = ""
	c.PortfolioName = ""
	c.ClearAccount()
}

// ClearAccount clears account context
func (c *Context) ClearAccount() {
	c.AccountID = ""
	c.AccountName = ""
}

// SetOrganization sets the organization context
func (c *Context) SetOrganization(id, name string) {
	c.OrganizationID = id
	c.OrganizationName = name
	// Clear dependent context
	c.ClearLedger()
}

// SetLedger sets the ledger context
func (c *Context) SetLedger(id, name string) {
	c.LedgerID = id
	c.LedgerName = name
	// Clear dependent context
	c.ClearPortfolio()
	c.ClearAccount()
}

// SetPortfolio sets the portfolio context
func (c *Context) SetPortfolio(id, name string) {
	c.PortfolioID = id
	c.PortfolioName = name
	// Clear dependent context
	c.ClearAccount()
}

// SetAccount sets the account context
func (c *Context) SetAccount(id, name string) {
	c.AccountID = id
	c.AccountName = name
}

// GetPrompt returns a formatted prompt showing the current context
func (c *Context) GetPrompt() string {
	parts := []string{}

	if c.OrganizationName != "" {
		parts = append(parts, c.OrganizationName)
	} else if c.OrganizationID != "" {
		parts = append(parts, "org:"+truncateID(c.OrganizationID))
	}

	if c.LedgerName != "" {
		parts = append(parts, c.LedgerName)
	} else if c.LedgerID != "" {
		parts = append(parts, "led:"+truncateID(c.LedgerID))
	}

	if c.PortfolioName != "" {
		parts = append(parts, c.PortfolioName)
	} else if c.PortfolioID != "" {
		parts = append(parts, "prt:"+truncateID(c.PortfolioID))
	}

	if c.AccountName != "" {
		parts = append(parts, c.AccountName)
	} else if c.AccountID != "" {
		parts = append(parts, "acc:"+truncateID(c.AccountID))
	}

	if len(parts) > 0 {
		return fmt.Sprintf("mdz [%s]> ", strings.Join(parts, "/"))
	}

	return "mdz> "
}

// String returns a string representation of the context
func (c *Context) String() string {
	parts := []string{}

	if c.OrganizationID != "" {
		parts = append(parts, fmt.Sprintf("Organization: %s (%s)", c.OrganizationName, c.OrganizationID))
	}

	if c.LedgerID != "" {
		parts = append(parts, fmt.Sprintf("Ledger: %s (%s)", c.LedgerName, c.LedgerID))
	}

	if c.PortfolioID != "" {
		parts = append(parts, fmt.Sprintf("Portfolio: %s (%s)", c.PortfolioName, c.PortfolioID))
	}

	if c.AccountID != "" {
		parts = append(parts, fmt.Sprintf("Account: %s (%s)", c.AccountName, c.AccountID))
	}

	if len(parts) == 0 {
		return "No context set"
	}

	return strings.Join(parts, "\n")
}

// truncateID truncates a UUID to show only the first 8 characters
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
