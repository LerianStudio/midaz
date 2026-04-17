// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package out

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

type grpcNoopLogger struct{}

func (grpcNoopLogger) Info(_ ...any)                                     {}
func (grpcNoopLogger) Infof(_ string, _ ...any)                          {}
func (grpcNoopLogger) Infoln(_ ...any)                                   {}
func (grpcNoopLogger) Error(_ ...any)                                    {}
func (grpcNoopLogger) Errorf(_ string, _ ...any)                         {}
func (grpcNoopLogger) Errorln(_ ...any)                                  {}
func (grpcNoopLogger) Warn(_ ...any)                                     {}
func (grpcNoopLogger) Warnf(_ string, _ ...any)                          {}
func (grpcNoopLogger) Warnln(_ ...any)                                   {}
func (grpcNoopLogger) Debug(_ ...any)                                    {}
func (grpcNoopLogger) Debugf(_ string, _ ...any)                         {}
func (grpcNoopLogger) Debugln(_ ...any)                                  {}
func (grpcNoopLogger) Fatal(_ ...any)                                    {}
func (grpcNoopLogger) Fatalf(_ string, _ ...any)                         {}
func (grpcNoopLogger) Fatalln(_ ...any)                                  {}
func (grpcNoopLogger) WithFields(_ ...any) libLog.Logger                 { return grpcNoopLogger{} }
func (grpcNoopLogger) WithDefaultMessageTemplate(_ string) libLog.Logger { return grpcNoopLogger{} }
func (grpcNoopLogger) Sync() error                                       { return nil }

func TestNewBalanceGRPC_LazyDial(t *testing.T) {
	t.Parallel()

	// grpc.NewClient performs a lazy dial and does not fail for arbitrary
	// addresses, so the constructor should return a ready-to-use repository.
	conn := &mgrpc.GRPCConnection{
		Addr:   "localhost:12345",
		Logger: grpcNoopLogger{},
	}

	r, err := NewBalanceGRPC(conn)
	require.NoError(t, err)
	require.NotNil(t, r)
}

func TestNewBalanceAdapter_LazyDial(t *testing.T) {
	t.Parallel()

	conn := &mgrpc.GRPCConnection{
		Addr:   "localhost:12345",
		Logger: grpcNoopLogger{},
	}

	a, err := NewBalanceAdapter(conn)
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestBalanceGRPC_CheckHealth_DelegatesToConnection(t *testing.T) {
	t.Parallel()

	conn := &mgrpc.GRPCConnection{
		Addr:   "localhost:12345",
		Logger: grpcNoopLogger{},
	}

	r, err := NewBalanceGRPC(conn)
	require.NoError(t, err)

	// CheckHealth delegates to conn.CheckHealth which fails fast because the
	// conn has no active channel state; we only need the code path to execute.
	err = r.CheckHealth(context.Background())
	// The underlying library may return different error values depending on
	// state transitions; we only care that it returned (not panicked).
	assert.Error(t, err)
}

func TestBalanceAdapter_CheckHealth_Delegates(t *testing.T) {
	t.Parallel()

	conn := &mgrpc.GRPCConnection{
		Addr:   "localhost:12345",
		Logger: grpcNoopLogger{},
	}

	a, err := NewBalanceAdapter(conn)
	require.NoError(t, err)

	err = a.CheckHealth(context.Background())
	assert.Error(t, err)
}

func TestBalanceAdapter_DeleteAllBalancesByAccountID_PropagatesError(t *testing.T) {
	t.Parallel()

	conn := &mgrpc.GRPCConnection{
		Addr:   "localhost:12345",
		Logger: grpcNoopLogger{},
	}

	a, err := NewBalanceAdapter(conn)
	require.NoError(t, err)

	err = a.DeleteAllBalancesByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(), "req-1")
	assert.Error(t, err)
}

func TestBalanceAdapter_CreateBalanceSync_PropagatesError(t *testing.T) {
	t.Parallel()

	conn := &mgrpc.GRPCConnection{
		Addr:   "localhost:12345",
		Logger: grpcNoopLogger{},
	}

	a, err := NewBalanceAdapter(conn)
	require.NoError(t, err)

	_, err = a.CreateBalanceSync(context.Background(), mmodel.CreateBalanceInput{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		AccountID:      uuid.New(),
	})
	assert.Error(t, err)
}
