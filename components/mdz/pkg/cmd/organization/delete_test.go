package organization

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func TestNewCmdOrganizationDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockOrganization(ctrl)

	orgFactory := factoryOrganizationDelete{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoOrganization: mockRepo,
		organizationID:   "123",
	}

	cmd := newCmdOrganizationDelete(&orgFactory)
	cmd.SetArgs([]string{
		"--organization-id", "123",
	})

	mockRepo.EXPECT().Delete(gomock.Any()).Return(nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The Organization 123 has been successfully deleted.")
}
