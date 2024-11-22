package metadata

import (
	"context"

	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
)

// Repository provides an interface for operations related on mongodb a metadata entities.
//
//go:generate mockgen --destination=../../mock/metadata/metadata_repository_mock.go --package=metadata . Repository
type Repository interface {
	Create(ctx context.Context, collection string, metadata *Metadata) error
	FindList(ctx context.Context, collection string, filter commonHTTP.QueryHeader) ([]*Metadata, error)
	FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)
	Update(ctx context.Context, collection, id string, metadata map[string]any) error
	Delete(ctx context.Context, collection, id string) error
}
