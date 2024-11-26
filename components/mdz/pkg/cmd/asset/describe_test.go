package asset

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

func Test_newCmdAssetDescribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockAsset(ctrl)

	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
	assetID := "01930365-4d46-7a09-a503-b932714f85af"

	ledFactory := factoryAssetDescribe{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoAsset:      mockRepo,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AssetID:        assetID,
		Out:            "",
		JSON:           false,
	}

	metadata := map[string]any{
		"chave1": "valor1",
		"chave2": 2,
		"chave3": true,
	}

	cmd := newCmdAssetDescribe(&ledFactory)
	cmd.SetArgs([]string{
		"--ledger-id", ledgerID,
		"--organization-id", organizationID,
		"--asset-id", assetID,
	})

	item := mmodel.Asset{
		ID:             assetID,
		Name:           "2Real",
		Type:           "commodity",
		Code:           "DOP",
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

	mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any()).Return(&item, nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
	golden.AssertBytes(t, output, "output_describe.golden")
}
