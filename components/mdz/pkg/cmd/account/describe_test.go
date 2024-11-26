package account

import (
	"bytes"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"gotest.tools/golden"
)

func Test_newCmdAccountDescribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockAccount(ctrl)

	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
	portfolioID := "01930365-4d46-7a09-a503-b932714f85af"
	accountID := "01933f96-ed04-7c57-be5b-c091388830f8"

	ledFactory := factoryAccountDescribe{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoAccount:    mockRepo,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		PortfolioID:    portfolioID,
		AccountID:      accountID,
		Out:            "",
		JSON:           false,
	}

	metadata := map[string]any{
		"chave1": "valor1",
		"chave2": 2,
		"chave3": true,
	}

	cmd := newCmdAccountDescribe(&ledFactory)
	cmd.SetArgs([]string{
		"--ledger-id", ledgerID,
		"--organization-id", organizationID,
		"--portfolio-id", portfolioID,
		"--account-id", accountID,
	})

	item := mmodel.Account{
		ID:             accountID,
		Name:           "2Real",
		Type:           "commodity",
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: ptr.StringPtr("Teste Ledger"),
		},
		CreatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
		UpdatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
		DeletedAt: nil,
		Metadata:  metadata,
	}

	mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&item, nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
	golden.AssertBytes(t, output, "output_describe.golden")
}
