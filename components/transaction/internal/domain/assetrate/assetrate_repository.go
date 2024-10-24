package assetrate

import "context"

// Repository provides an interface for assetrate template entities.
//
//go:generate mockgen --destination=../../gen/mock/assetrate/assetrate_mock.go --package=mock . Repository
type Repository interface {
	Create(ctx context.Context, assetRate *AssetRate) (*AssetRate, error)
}
