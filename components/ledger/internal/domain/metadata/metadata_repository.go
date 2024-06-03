package metadata

import (
	"context"

	"github.com/LerianStudio/midaz/common"
)

// Repository provides an interface for operations related on mongodb a metadata entities.
//
//go:generate mockgen --destination=../../gen/mock/metadata/metadata_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, collection string, metadata *Metadata) error
	FindList(ctx context.Context, collection string, filter common.QueryHeader) ([]*Metadata, error)
	FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)
	Update(ctx context.Context, collection, id string, metadata map[string]any) error
	Delete(ctx context.Context, collection, id string) error
}
