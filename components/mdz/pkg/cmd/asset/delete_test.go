package asset

import (
	"bytes"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdAssetDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockAsset(ctrl)

	orgFactory := factoryAssetDelete{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoAsset:      mockRepo,
		OrganizationID: "321",
		LedgerID:       "123",
		AssetID:        "444",
	}

	cmd := newCmdAssetDelete(&orgFactory)
	cmd.SetArgs([]string{
		"--organization-id", "321",
		"--ledger-id", "123",
		"--asset-id", "444",
	})

	mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The Asset ID 444 has been successfully deleted.")
}
