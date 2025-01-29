package cluster

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// ClusterPostgreSQLModel represents the entity Cluster into SQL context in Database
type ClusterPostgreSQLModel struct {
	ID                string
	Name              string
	LedgerID          string
	OrganizationID    string
	Status            string
	StatusDescription *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts an ClusterPostgreSQLModel to entity.Cluster
func (t *ClusterPostgreSQLModel) ToEntity() *mmodel.Cluster {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	cluster := &mmodel.Cluster{
		ID:             t.ID,
		Name:           t.Name,
		LedgerID:       t.LedgerID,
		OrganizationID: t.OrganizationID,
		Status:         status,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		DeletedAt:      nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		cluster.DeletedAt = &deletedAtCopy
	}

	return cluster
}

// FromEntity converts an entity.Cluster to ClusterPostgreSQLModel
func (t *ClusterPostgreSQLModel) FromEntity(cluster *mmodel.Cluster) {
	*t = ClusterPostgreSQLModel{
		ID:                pkg.GenerateUUIDv7().String(),
		Name:              cluster.Name,
		LedgerID:          cluster.LedgerID,
		OrganizationID:    cluster.OrganizationID,
		Status:            cluster.Status.Code,
		StatusDescription: cluster.Status.Description,
		CreatedAt:         cluster.CreatedAt,
		UpdatedAt:         cluster.UpdatedAt,
	}

	if cluster.DeletedAt != nil {
		deletedAtCopy := *cluster.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
