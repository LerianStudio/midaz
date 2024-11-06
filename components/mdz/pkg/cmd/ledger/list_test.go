package ledger

import (
	"bytes"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"gotest.tools/golden"
)

func Test_newCmdLedgerList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockLedger(ctrl)

	ledFactory := factoryLedgerList{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoLedger: mockRepo,
	}

	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"

	cmd := newCmdLedgerList(&ledFactory)
	cmd.SetArgs([]string{"--organization-id", organizationID})

	list := mmodel.Ledgers{
		Page:  1,
		Limit: 5,
		Items: []mmodel.Ledger{
			{
				ID:             "0192e362-b270-7158-a647-7a59e4e26a27",
				Name:           "Ankunding - Paucek",
				OrganizationID: organizationID,
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste Ledger"),
				},
				CreatedAt: time.Date(2024, 10, 31, 16, 22, 29, 232078000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 16, 22, 29, 232078000, time.UTC),
				DeletedAt: nil,
				Metadata: map[string]any{
					"bitcoin": "3HH89s3LPALardk1jLt2PcjAJng",
					"chave":   "metadata_chave",
					"boolean": true,
				},
			},
			{
				ID:             "0192e258-2c81-7e37-b6ba-a2007495c652",
				Name:           "Zieme - Mante",
				OrganizationID: organizationID,
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste Ledger"),
				},
				CreatedAt: time.Date(2024, 10, 31, 11, 31, 22, 369928000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 11, 31, 22, 369928000, time.UTC),
				DeletedAt: nil,
				Metadata: map[string]any{
					"bitcoin": "329aaP47xTc8hQxXB92896U2RBXGEt",
					"chave":   "metadata_chave",
					"boolean": true,
				},
			},
			{
				ID:             "0192e257-f5c0-7687-8534-303bae7aa4aa",
				Name:           "Lang LLC",
				OrganizationID: organizationID,
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: nil,
				},
				CreatedAt: time.Date(2024, 10, 31, 11, 31, 8, 352409000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 11, 31, 8, 352409000, time.UTC),
				DeletedAt: nil,
			},
			{
				ID:             "0192e251-328d-7390-99f5-5c54980115ed",
				Name:           "Romaguera and Sons",
				OrganizationID: organizationID,
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste Ledger"),
				},
				CreatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
				DeletedAt: nil,
				Metadata: map[string]any{
					"bitcoin": "1iR2KqpxRFjLsPUpWmpADMC7piRNsMAAjq",
					"chave":   "metadata_chave",
					"boolean": false,
				},
			},
		},
	}

	mockRepo.EXPECT().Get(organizationID, gomock.Any(), gomock.Any()).
		Return(&list, nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
	golden.AssertBytes(t, output, "output_list.golden")
}
