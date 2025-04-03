//go:build integration
// +build integration

package integration

import (
	"fmt"
	"math/rand"
	"os/exec"
	"testing"
	"time"

	"github.com/icrowley/fake"
	"gotest.tools/golden"
)

// \1 performs an operation
func TestMDZ(t *testing.T) {
	var stdout string

	stdout, _ = cmdRun(t, exec.Command("mdz", "login",
		"--username", "user_john",
		"--password", "Lerian@123",
	))

	golden.AssertBytes(t, []byte(stdout), "out_login_flags.golden")

	stdout, _ = cmdRun(t, exec.Command("mdz", "organization", "create",
		"--legal-name", "Soul LLCT",
		"--doing-business-as", "The ledger.io",
		"--legal-document", "48784548000104",
		"--code", "ACTIVE",
		"--description", "Test Ledger",
		"--line1", "Av Santso",
		"--line2", "VJ 222",
		"--zip-code", "04696040",
		"--city", "West",
		"--state", "VJ",
		"--country", "MG",
		"--metadata", `{"chave1": "valor1", "chave2": 2,  "chave3": true}`,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "organization", "list"))

	organizationID := getIDListOutput(t, stdout)

	stdout, _ = cmdRun(t, exec.Command("mdz", "organization", "describe",
		"--organization-id", organizationID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "organization", "update",
		"--organization-id", organizationID,
		"--legal-name", fake.FirstName(),
		"--doing-business-as", fake.Word(),
		"--country", "BR",
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "ledger", "create",
		"--organization-id", organizationID,
		"--name", fake.FirstName(),
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "ledger", "list",
		"--organization-id", organizationID,
	))

	ledgerID := getIDListOutput(t, stdout)

	stdout, _ = cmdRun(t, exec.Command("mdz", "ledger", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "ledger", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", fake.FirstName(),
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "asset", "create",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", fake.FirstName(),
		"--code", "BRL",
		"--type", "currency",
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "asset", "list",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	))

	assetID := getIDListOutput(t, stdout)

	stdout, _ = cmdRun(t, exec.Command("mdz", "asset", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--asset-id", assetID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "asset", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--asset-id", assetID,
		"--name", fake.FirstName(),
	))

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomNumber := r.Intn(999999)

	stdout, _ = cmdRun(t, exec.Command("mdz", "portfolio", "create",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", fake.FirstName(),
		"--entity-id", fmt.Sprint(randomNumber),
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "portfolio", "list",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	))

	portfolioID := getIDListOutput(t, stdout)

	stdout, _ = cmdRun(t, exec.Command("mdz", "portfolio", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "portfolio", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
		"--name", fake.FirstName(),
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "segment", "create",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", fake.FirstName(),
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "segment", "list",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	))

	segmentID := getIDListOutput(t, stdout)

	stdout, _ = cmdRun(t, exec.Command("mdz", "segment", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--segment-id", segmentID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "segment", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--segment-id", segmentID,
		"--name", fake.FirstName(),
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "account", "create",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
		"--segment-id", segmentID,
		"--name", fake.FirstName(),
		"--asset-code", "BRL",
		"--type", "creditCard",
		"--alias", "@wallet_luffy",
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "account", "list",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
	))

	accountID := getIDListOutput(t, stdout)

	stdout, _ = cmdRun(t, exec.Command("mdz", "account", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
		"--account-id", accountID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "account", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
		"--account-id", accountID,
		"--segment-id", segmentID,
		"--name", fake.FirstName(),
	))

	t.Log("organization ID: ", organizationID)
	t.Log("ledger ID: ", ledgerID)
	t.Log("asset ID: ", assetID)
	t.Log("portfolio ID: ", portfolioID)
	t.Log("segment ID: ", segmentID)
	t.Log("account ID: ", accountID)

	stdout, _ = cmdRun(t, exec.Command("mdz", "account", "delete",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
		"--account-id", accountID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "asset", "delete",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--asset-id", assetID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "segment", "delete",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--segment-id", segmentID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "portfolio", "delete",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "ledger", "delete",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	))

	stdout, _ = cmdRun(t, exec.Command("mdz", "organization", "delete",
		"--organization-id", organizationID,
	))
}
