package mmodel

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewHolder_ValidInputs_NaturalPerson(t *testing.T) {
	id := uuid.New()

	holder := NewHolder(id, "John Doe", "91315026015", HolderTypeNaturalPerson)

	assert.Equal(t, &id, holder.ID)
	assert.Equal(t, "John Doe", *holder.Name)
	assert.Equal(t, "91315026015", *holder.Document)
	assert.Equal(t, HolderTypeNaturalPerson, *holder.Type)
	assert.False(t, holder.CreatedAt.IsZero())
	assert.False(t, holder.UpdatedAt.IsZero())
}

func TestNewHolder_ValidInputs_LegalPerson(t *testing.T) {
	id := uuid.New()

	holder := NewHolder(id, "Acme Corp", "12345678000190", HolderTypeLegalPerson)

	assert.Equal(t, &id, holder.ID)
	assert.Equal(t, "Acme Corp", *holder.Name)
	assert.Equal(t, "12345678000190", *holder.Document)
	assert.Equal(t, HolderTypeLegalPerson, *holder.Type)
	assert.False(t, holder.CreatedAt.IsZero())
	assert.False(t, holder.UpdatedAt.IsZero())
}

func TestNewHolder_NilUUID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.Nil, "John Doe", "91315026015", HolderTypeNaturalPerson)
	}, "should panic with nil UUID")
}

func TestNewHolder_EmptyName_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.New(), "", "91315026015", HolderTypeNaturalPerson)
	}, "should panic with empty name")
}

func TestNewHolder_EmptyDocument_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.New(), "John Doe", "", HolderTypeNaturalPerson)
	}, "should panic with empty document")
}

func TestNewHolder_EmptyHolderType_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.New(), "John Doe", "91315026015", "")
	}, "should panic with empty holder type")
}

func TestNewHolder_InvalidHolderType_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.New(), "John Doe", "91315026015", "INVALID_TYPE")
	}, "should panic with invalid holder type")
}
