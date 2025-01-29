package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

type Cluster interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateClusterInput) (*mmodel.Cluster, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Clusters, error)
	GetByID(organizationID, ledgerID, clusterID string) (*mmodel.Cluster, error)
	Update(organizationID, ledgerID, clusterID string, inp mmodel.UpdateClusterInput) (*mmodel.Cluster, error)
	Delete(organizationID, ledgerID, clusterID string) error
}
