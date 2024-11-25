package integration

import (
	"bytes"
	"fmt"
	"math/rand"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/icrowley/fake"
	"gotest.tools/golden"
)

// cmdRun run command and check error
func cmdRun(t *testing.T, cmd *exec.Cmd) (string, string) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Error executing command: %v\nStderr: %s", err, stderr.String())
	}

	return stdout.String(), stderr.String()
}

// getIDListOutput get id list command
func getIDListOutput(t *testing.T, stdout string) string {
	re := regexp.MustCompile(`[0-9a-fA-F-]{36}`)
	id := re.FindString(stdout)
	if id == "" {
		t.Fatal("No ID found in output")
	}

	return id
}

func TestMDZ(t *testing.T) {
	var stdout, stderr string

	cmdLogin := exec.Command("mdz", "login",
		"--username", "user_john",
		"--password", "Lerian@123",
	)
	stdout, stderr = cmdRun(t, cmdLogin)

	golden.AssertBytes(t, []byte(stdout), "out_login_flags.golden")

	cmdOrganizationCreate := exec.Command("mdz", "organization", "create",
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
	)
	stdout, stderr = cmdRun(t, cmdOrganizationCreate)

	cmdOrganizationList := exec.Command("mdz", "organization", "list")
	stdout, stderr = cmdRun(t, cmdOrganizationList)

	organizationID := getIDListOutput(t, stdout)
	cmdOrganizationDescribe := exec.Command("mdz", "organization", "describe",
		"--organization-id", organizationID,
	)

	stdout, stderr = cmdRun(t, cmdOrganizationDescribe)

	cmdOrganizationUpdate := exec.Command("mdz", "organization", "update",
		"--organization-id", organizationID,
		"--legal-name", fake.FirstName(),
		"--doing-business-as", fake.Word(),
		"--country", "BR",
	)
	stdout, stderr = cmdRun(t, cmdOrganizationUpdate)

	cmdLedgerCreate := exec.Command("mdz", "ledger", "create",
		"--organization-id", organizationID,
		"--name", fake.FirstName(),
	)
	stdout, stderr = cmdRun(t, cmdLedgerCreate)

	cmdLedgerList := exec.Command("mdz", "ledger", "list",
		"--organization-id", organizationID,
	)
	stdout, stderr = cmdRun(t, cmdLedgerList)

	ledgerID := getIDListOutput(t, stdout)

	cmdLedgerDescribe := exec.Command("mdz", "ledger", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	)
	stdout, stderr = cmdRun(t, cmdLedgerDescribe)

	cmdLedgerUpdate := exec.Command("mdz", "ledger", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", fake.FirstName(),
	)
	stdout, stderr = cmdRun(t, cmdLedgerUpdate)

	cmdAssetCreate := exec.Command("mdz", "asset", "create",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", fake.FirstName(),
		"--code", "AOA",
		"--type", "commodity",
	)

	stdout, stderr = cmdRun(t, cmdAssetCreate)

	cmdAssetList := exec.Command("mdz", "asset", "list",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	)
	stdout, stderr = cmdRun(t, cmdAssetList)

	assetID := getIDListOutput(t, stdout)

	cmdAssetDescribe := exec.Command("mdz", "asset", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--asset-id", assetID,
	)
	stdout, stderr = cmdRun(t, cmdAssetDescribe)

	cmdAssetUpdate := exec.Command("mdz", "asset", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--asset-id", assetID,
		"--name", fake.FirstName(),
	)
	stdout, stderr = cmdRun(t, cmdAssetUpdate)

	cmdAssetDelete := exec.Command("mdz", "asset", "delete",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--asset-id", assetID,
	)
	stdout, stderr = cmdRun(t, cmdAssetDelete)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomNumber := r.Intn(999999)

	cmdPortfolioCreate := exec.Command("mdz", "portfolio", "create",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", fake.FirstName(),
		"--entity-id", fmt.Sprint(randomNumber),
	)

	stdout, stderr = cmdRun(t, cmdPortfolioCreate)

	cmdPortfolioList := exec.Command("mdz", "portfolio", "list",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	)
	stdout, stderr = cmdRun(t, cmdPortfolioList)

	portfolioID := getIDListOutput(t, stdout)

	cmdPortfolioDescribe := exec.Command("mdz", "portfolio", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
	)
	stdout, stderr = cmdRun(t, cmdPortfolioDescribe)

	cmdPortfolioUpdate := exec.Command("mdz", "portfolio", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--portfolio-id", portfolioID,
		"--name", fake.FirstName(),
	)
	stdout, stderr = cmdRun(t, cmdPortfolioUpdate)

	cmdProductCreate := exec.Command("mdz", "product", "create",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", fake.FirstName(),
	)

	stdout, stderr = cmdRun(t, cmdProductCreate)

	cmdProductList := exec.Command("mdz", "product", "list",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
	)
	stdout, stderr = cmdRun(t, cmdProductList)

	productID := getIDListOutput(t, stdout)

	cmdProductDescribe := exec.Command("mdz", "product", "describe",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--product-id", productID,
	)
	stdout, stderr = cmdRun(t, cmdProductDescribe)

	cmdProductUpdate := exec.Command("mdz", "product", "update",
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--product-id", productID,
		"--name", fake.FirstName(),
	)
	stdout, stderr = cmdRun(t, cmdProductUpdate)

	fmt.Println(stdout)
	fmt.Println(stderr)
}
