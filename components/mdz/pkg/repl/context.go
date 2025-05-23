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

// String returns a formatted string representation of the context
func (c *Context) String() string {
	if c.OrganizationID == "" && c.LedgerID == "" && c.PortfolioID == "" && c.AccountID == "" {
		return `🏗️  No context set

Getting Started:
  1. Type 'organization list' to see available organizations
  2. Use 'use organization <id>' to set organization context
  3. Continue with 'ledger list' and so on...

💡 Tip: Commands will auto-prompt for missing context!`
	}

	var output strings.Builder

	output.WriteString("📍 Current Context\n")
	output.WriteString("================\n\n")

	// Organization
	if c.OrganizationID != "" {
		name := c.OrganizationName
		if name == "" {
			name = "Unnamed"
		}

		output.WriteString(fmt.Sprintf("🏢 Organization: %s\n", name))
		output.WriteString(fmt.Sprintf("   ID: %s\n\n", c.OrganizationID))
	}

	// Ledger
	if c.LedgerID != "" {
		name := c.LedgerName
		if name == "" {
			name = "Unnamed"
		}

		output.WriteString(fmt.Sprintf("📊 Ledger: %s\n", name))
		output.WriteString(fmt.Sprintf("   ID: %s\n\n", c.LedgerID))
	}

	// Portfolio
	if c.PortfolioID != "" {
		name := c.PortfolioName
		if name == "" {
			name = "Unnamed"
		}

		output.WriteString(fmt.Sprintf("💼 Portfolio: %s\n", name))
		output.WriteString(fmt.Sprintf("   ID: %s\n\n", c.PortfolioID))
	}

	// Account
	if c.AccountID != "" {
		name := c.AccountName
		if name == "" {
			name = "Unnamed"
		}

		output.WriteString(fmt.Sprintf("🎯 Account: %s\n", name))
		output.WriteString(fmt.Sprintf("   ID: %s\n\n", c.AccountID))
	}

	// Add suggestions based on context
	output.WriteString("💡 Suggested Actions:\n")

	if c.AccountID != "" {
		output.WriteString("   • balance list\n")
		output.WriteString("   • operation list\n")
		output.WriteString("   • transaction create\n")
	} else if c.LedgerID != "" {
		output.WriteString("   • account list\n")
		output.WriteString("   • portfolio list\n")
		output.WriteString("   • asset list\n")
	} else if c.OrganizationID != "" {
		output.WriteString("   • ledger list\n")
		output.WriteString("   • asset list\n")
	}

	return output.String()
}

// truncateID truncates a UUID to show only the first 8 characters
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}

	return id
}
