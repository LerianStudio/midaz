package asset

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdAssetDelete(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
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
		assert.Contains(t, output, "The Asset 444 has been successfully deleted.")
	})

	t.Run("happy path no informing flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAsset(ctrl)

		orgFactory := factoryAssetDelete{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoAsset: mockRepo,
		}

		orgFactory.tuiInput = func(message string) (string, error) {
			return "01933f96-ed04-7c57-be5b-c091388830f8", nil
		}

		cmd := newCmdAssetDelete(&orgFactory)
		cmd.SetArgs([]string{})

		mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Asset 01933f96-ed04-7c57-be5b-c091388830f8 has been successfully deleted.")
	})
}
