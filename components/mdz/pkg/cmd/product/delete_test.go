package product

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdProductDelete(t *testing.T) {
	t.Run("with flags", func(t *testing.T) {
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
			"--cluster-id", "444",
		})

		mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := factory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Product 444 has been successfully deleted.")
	})

	t.Run("no flags", func(t *testing.T) {
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

		factory.tuiInput = func(message string) (string, error) {
			return "444", nil
		}

		cmd := newCmdProductDelete(&factory)
		cmd.SetArgs([]string{})

		mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := factory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Product 444 has been successfully deleted.")
	})
}
