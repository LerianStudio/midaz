package product

import (
	"bytes"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func Test_newCmdProductDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockProduct(ctrl)

	factory := factoryProductDelete{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoProduct:    mockRepo,
		OrganizationID: "321",
		LedgerID:       "123",
		ProductID:      "444",
	}

	cmd := newCmdProductDelete(&factory)
	cmd.SetArgs([]string{
		"--organization-id", "321",
		"--ledger-id", "123",
		"--product-id", "444",
	})

	mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := factory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The Product ID 444 has been successfully deleted.")
}
