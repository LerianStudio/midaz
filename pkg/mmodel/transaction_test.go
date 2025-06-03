package mmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransaction_IsEmpty(t *testing.T) {
	tests := []struct {
		name        string
		transaction Transaction
		want        bool
	}{
		{
			name: "empty transaction",
			transaction: Transaction{
				Send: Send{
					Asset: "",
					Value: "",
				},
			},
			want: true,
		},
		{
			name: "transaction with asset only",
			transaction: Transaction{
				Send: Send{
					Asset: "BRL",
					Value: "",
				},
			},
			want: false,
		},
		{
			name: "transaction with value only",
			transaction: Transaction{
				Send: Send{
					Asset: "",
					Value: "1000",
				},
			},
			want: false,
		},
		{
			name: "complete transaction",
			transaction: Transaction{
				Send: Send{
					Asset: "BRL",
					Value: "1000",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.transaction.IsEmpty()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFromTo_SplitAlias(t *testing.T) {
	tests := []struct {
		name   string
		fromTo FromTo
		want   string
	}{
		{
			name: "alias without index",
			fromTo: FromTo{
				AccountAlias: "@person1",
			},
			want: "@person1",
		},
		{
			name: "alias with index",
			fromTo: FromTo{
				AccountAlias: "1#@person1",
			},
			want: "@person1",
		},
		{
			name: "empty alias",
			fromTo: FromTo{
				AccountAlias: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fromTo.SplitAlias()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFromTo_ConcatAlias(t *testing.T) {
	tests := []struct {
		name   string
		fromTo FromTo
		index  int
		want   string
	}{
		{
			name: "concat with index 1",
			fromTo: FromTo{
				AccountAlias: "@person1",
			},
			index: 1,
			want:  "1#@person1",
		},
		{
			name: "concat with index 0",
			fromTo: FromTo{
				AccountAlias: "@person1",
			},
			index: 0,
			want:  "0#@person1",
		},
		{
			name: "concat with empty alias",
			fromTo: FromTo{
				AccountAlias: "",
			},
			index: 1,
			want:  "1#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fromTo.ConcatAlias(tt.index)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRate_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		rate Rate
		want bool
	}{
		{
			name: "completely empty rate",
			rate: Rate{
				ExternalID: "",
				From:       "",
				To:         "",
				Value:      "",
			},
			want: true,
		},
		{
			name: "rate with only ExternalID",
			rate: Rate{
				ExternalID: "00000000-0000-0000-0000-000000000000",
				From:       "",
				To:         "",
				Value:      "",
			},
			want: false,
		},
		{
			name: "rate with only From",
			rate: Rate{
				ExternalID: "",
				From:       "BRL",
				To:         "",
				Value:      "",
			},
			want: false,
		},
		{
			name: "rate with only To",
			rate: Rate{
				ExternalID: "",
				From:       "",
				To:         "USDe",
				Value:      "",
			},
			want: false,
		},
		{
			name: "rate with only Value",
			rate: Rate{
				ExternalID: "",
				From:       "",
				To:         "",
				Value:      "1000",
			},
			want: false,
		},
		{
			name: "complete rate",
			rate: Rate{
				ExternalID: "00000000-0000-0000-0000-000000000000",
				From:       "BRL",
				To:         "USDe",
				Value:      "1000",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rate.IsEmpty()
			assert.Equal(t, tt.want, got)
		})
	}
}
