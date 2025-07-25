package mmodel

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// SettingsMongoDBModel represents the settings into mongodb context
type SettingsMongoDBModel struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	OrganizationID  string             `bson:"organization_id"`
	LedgerID        string             `bson:"ledger_id"`
	ApplicationName string             `bson:"application_name"`
	Settings        JSON               `bson:"settings"`
	Enabled         bool               `bson:"enabled"`
	CreatedAt       time.Time          `bson:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at"`
	DeletedAt       *time.Time         `bson:"deleted_at"`
}

// Settings is a struct designed to encapsulate payload data
type Settings struct {
	ID              primitive.ObjectID `json:"id"`
	OrganizationID  string             `json:"organization_id"`
	LedgerID        string             `json:"ledger_id"`
	ApplicationName string             `json:"application_name"`
	Settings        JSON               `json:"settings"`
	Enabled         bool               `json:"enabled"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	DeletedAt       *time.Time         `json:"deleted_at"`
}

// JSON document to save on mongodb
type JSON map[string]any

// Value return marshall value data
func (s JSON) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan unmarshall value data
func (s *JSON) Scan(value any) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &s)
}

// ToEntity is a func that convert SettingsMongoDBModel to Settings dto.
func (smm *SettingsMongoDBModel) ToEntity() *Settings {
	return &Settings{
		ID:              smm.ID,
		OrganizationID:  smm.OrganizationID,
		LedgerID:        smm.LedgerID,
		ApplicationName: smm.ApplicationName,
		Settings:        smm.Settings,
		Enabled:         smm.Enabled,
		CreatedAt:       smm.CreatedAt,
		UpdatedAt:       smm.UpdatedAt,
		DeletedAt:       smm.DeletedAt,
	}
}

// ToDTO is a func that convert Settings dto to SettingsMongoDBModel
func (settings *Settings) ToDTO() *SettingsMongoDBModel {
	return &SettingsMongoDBModel{
		ID:              settings.ID,
		OrganizationID:  settings.OrganizationID,
		LedgerID:        settings.LedgerID,
		ApplicationName: settings.ApplicationName,
		Settings:        settings.Settings,
		Enabled:         settings.Enabled,
		CreatedAt:       settings.CreatedAt,
		UpdatedAt:       settings.UpdatedAt,
		DeletedAt:       settings.DeletedAt,
	}
}
