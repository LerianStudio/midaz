package pkg

import (
	"testing"

	"github.com/google/uuid"
)

func Test_GenerateUUIDv7(t *testing.T) {
	u := GenerateUUIDv7()
	if u.Version() != 7 {
		t.Errorf("Expected UUID version 7, but got version %d", u.Version())
	}
	if u.Variant() != uuid.RFC4122 {
		t.Errorf("Expected UUID variant RFC4122, but got variant %d", u.Variant())
	}
}
