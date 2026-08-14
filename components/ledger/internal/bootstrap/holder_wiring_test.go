// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHolderByIDReader stubs GetHolderByID for the Exists discrimination test.
type fakeHolderByIDReader struct {
	holder *mmodel.Holder
	err    error
}

func (f fakeHolderByIDReader) GetHolderByID(_ context.Context, _ string, _ uuid.UUID, _ bool) (*mmodel.Holder, error) {
	return f.holder, f.err
}

func TestHolderReaderAdapter_Exists(t *testing.T) {
	id := uuid.New()
	holder := &mmodel.Holder{ID: &id}

	infraErr := errors.New("mongo timeout")

	tests := []struct {
		name       string
		reader     fakeHolderByIDReader
		wantExists bool
		wantErr    bool
		wantErrIs  error
	}{
		{
			name:       "holder found",
			reader:     fakeHolderByIDReader{holder: holder},
			wantExists: true,
		},
		{
			name: "holder-not-found business error maps to (false, nil)",
			reader: fakeHolderByIDReader{err: pkg.EntityNotFoundError{
				Code: constant.ErrHolderNotFound.Error(),
			}},
			wantExists: false,
		},
		{
			name: "different EntityNotFoundError code propagates",
			reader: fakeHolderByIDReader{err: pkg.EntityNotFoundError{
				Code: constant.ErrOrganizationIDNotFound.Error(),
			}},
			wantExists: false,
			wantErr:    true,
		},
		{
			name:       "infrastructure error propagates",
			reader:     fakeHolderByIDReader{err: infraErr},
			wantExists: false,
			wantErr:    true,
			wantErrIs:  infraErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := holderReaderAdapter{service: tt.reader}

			exists, err := adapter.Exists(context.Background(), "org-1", id)

			assert.Equal(t, tt.wantExists, exists)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
		})
	}
}
