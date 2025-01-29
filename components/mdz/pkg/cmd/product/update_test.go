package product

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdProductUpdate(t *testing.T) {
	t.Run("with flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockProduct(ctrl)

		orgFactory := factoryProductUpdate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoProduct: mockRepo,
			tuiInput: func(message string) (string, error) {
				return "name", nil
			},
			flagsUpdate: flagsUpdate{
				Name:              "Test Organization",
				StatusCode:        "BLOCKED",
				StatusDescription: "Teste BLOCKED Ledger",
				Metadata:          "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
			},
		}

		cmd := newCmdProductUpdate(&orgFactory)
		cmd.SetArgs([]string{
			"--organization-id", "123",
			"--ledger-id", "321",
			"--cluster-id", "412",
			"--name", "Test Organization",
			"--status-code", "BLOCKED",
			"--status-description", "Teste BLOCKED Ledger",
			"--metadata", "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
		})

		metadata := map[string]any{
			"chave1": "valor1",
			"chave2": 2,
			"chave3": true,
		}

		gotOrg := &mmodel.Product{
			ID:   "412",
			Name: "Test Organization",
			Status: mmodel.Status{
				Code:        "BLOCKED",
				Description: ptr.StringPtr("Teste BLOCKED Ledger"),
			},
			Metadata: metadata,
		}

		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(gotOrg, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Product 412 has been successfully updated.")
	})

	t.Run("no flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockProduct(ctrl)

		orgFactory := factoryProductUpdate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoProduct: mockRepo,
			tuiInput: func(message string) (string, error) {
				return "name", nil
			},
			flagsUpdate: flagsUpdate{
				Name:              "Test Organization",
				StatusCode:        "BLOCKED",
				StatusDescription: "Teste BLOCKED Ledger",
				Metadata:          "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
			},
		}

		orgFactory.tuiInput = func(message string) (string, error) {
			return "412", nil
		}

		cmd := newCmdProductUpdate(&orgFactory)
		cmd.SetArgs([]string{
			"--name", "Test Organization",
			"--status-code", "BLOCKED",
			"--status-description", "Teste BLOCKED Ledger",
			"--metadata", "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
		})

		metadata := map[string]any{
			"chave1": "valor1",
			"chave2": 2,
			"chave3": true,
		}

		gotOrg := &mmodel.Product{
			ID:   "412",
			Name: "Test Organization",
			Status: mmodel.Status{
				Code:        "BLOCKED",
				Description: ptr.StringPtr("Teste BLOCKED Ledger"),
			},
			Metadata: metadata,
		}

		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(gotOrg, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Product 412 has been successfully updated.")
	})
}
