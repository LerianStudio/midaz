package mmodel

import (
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func TestAddress_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		address Address
		want    bool
	}{
		{
			name: "completely empty address",
			address: Address{
				Line1:   "",
				Line2:   nil,
				ZipCode: "",
				City:    "",
				State:   "",
				Country: "",
			},
			want: true,
		},
		{
			name: "address with only Line1",
			address: Address{
				Line1:   "123 Main St",
				Line2:   nil,
				ZipCode: "",
				City:    "",
				State:   "",
				Country: "",
			},
			want: false,
		},
		{
			name: "address with only Line2",
			address: Address{
				Line1:   "",
				Line2:   ptr.StringPtr("Apt 4B"),
				ZipCode: "",
				City:    "",
				State:   "",
				Country: "",
			},
			want: false,
		},
		{
			name: "address with only ZipCode",
			address: Address{
				Line1:   "",
				Line2:   nil,
				ZipCode: "12345",
				City:    "",
				State:   "",
				Country: "",
			},
			want: false,
		},
		{
			name: "address with only City",
			address: Address{
				Line1:   "",
				Line2:   nil,
				ZipCode: "",
				City:    "New York",
				State:   "",
				Country: "",
			},
			want: false,
		},
		{
			name: "address with only State",
			address: Address{
				Line1:   "",
				Line2:   nil,
				ZipCode: "",
				City:    "",
				State:   "NY",
				Country: "",
			},
			want: false,
		},
		{
			name: "address with only Country",
			address: Address{
				Line1:   "",
				Line2:   nil,
				ZipCode: "",
				City:    "",
				State:   "",
				Country: "US",
			},
			want: false,
		},
		{
			name: "complete address",
			address: Address{
				Line1:   "123 Main St",
				Line2:   ptr.StringPtr("Apt 4B"),
				ZipCode: "12345",
				City:    "New York",
				State:   "NY",
				Country: "US",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.address.IsEmpty()
			assert.Equal(t, tt.want, got)
		})
	}
}
