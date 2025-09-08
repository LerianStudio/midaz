//go:build integration

package redis

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "testing"
    "time"

    libCommons "github.com/LerianStudio/lib-commons/v2/commons"
    libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
    libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
    libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
    "errors"
    midazpkg "github.com/LerianStudio/midaz/v3/pkg"
    "github.com/LerianStudio/midaz/v3/pkg/constant"
    "github.com/LerianStudio/midaz/v3/pkg/mmodel"
    "github.com/google/uuid"
    "github.com/shopspring/decimal"
    tc "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
    rds "github.com/redis/go-redis/v9"
)

// startValkey spins up a Valkey container for tests and returns host:port and a cleanup func.
func startValkey(t *testing.T) (addr string, cleanup func()) {
    t.Helper()

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    t.Cleanup(cancel)

    req := tc.ContainerRequest{
        Image:        "valkey/valkey:latest",
        ExposedPorts: []string{"6379/tcp"},
        WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
    }

    c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: req, Started: true})
    if err != nil {
        t.Fatalf("failed to start valkey: %v", err)
    }

    host, err := c.Host(ctx)
    if err != nil {
        _ = c.Terminate(ctx)
        t.Fatalf("failed to get container host: %v", err)
    }
    port, err := c.MappedPort(ctx, "6379/tcp")
    if err != nil {
        _ = c.Terminate(ctx)
        t.Fatalf("failed to get container port: %v", err)
    }

    addr = fmt.Sprintf("%s:%s", host, port.Port())

    cleanup = func() {
        _ = c.Terminate(context.Background())
    }
    return addr, cleanup
}

func newRedisRepoForAddr(t *testing.T, addr string) *RedisConsumerRepository {
    t.Helper()
    rc := &libRedis.RedisConnection{
        Mode:    libRedis.ModeStandalone,
        Address: []string{addr},
        Logger:  &libLog.GoLogger{Level: libLog.ErrorLevel},
    }
    return NewConsumerRedis(rc)
}

func TestBatchApply_AllSucceed_Valkey(t *testing.T) {
    addr, cleanup := startValkey(t)
    defer cleanup()

    repo := newRedisRepoForAddr(t, addr)

    ctx := context.Background()
    orgID := uuid.New()
    ledgerID := uuid.New()

    // Prepare operations: one DEBIT (from), one CREDIT (to)
    keyFrom := libCommons.TransactionInternalKey(orgID, ledgerID, "alias_from")
    keyTo := libCommons.TransactionInternalKey(orgID, ledgerID, "alias_to")

    fromAmount := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(50), Operation: constant.DEBIT}
    toAmount := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(50), Operation: constant.CREDIT}

    fromBalance := mmodel.Balance{ // seed values for first-time creation
        ID:             uuid.New().String(),
        AccountID:      uuid.New().String(),
        Available:      decimal.NewFromInt(100),
        OnHold:         decimal.Zero,
        Version:        1,
        AccountType:    "deposit",
        AllowSending:   true,
        AllowReceiving: true,
        AssetCode:      "USD",
    }
    toBalance := mmodel.Balance{
        ID:             uuid.New().String(),
        AccountID:      uuid.New().String(),
        Available:      decimal.NewFromInt(0),
        OnHold:         decimal.Zero,
        Version:        1,
        AccountType:    "deposit",
        AllowSending:   true,
        AllowReceiving: true,
        AssetCode:      "USD",
    }

    keys := []string{keyFrom, keyTo}
    amounts := []libTransaction.Amount{fromAmount, toAmount}
    balances := []mmodel.Balance{fromBalance, toBalance}

    results, err := repo.AddSumBalancesAtomicRedis(ctx, keys, constant.CREATED, false, amounts, balances)
    if err != nil {
        t.Fatalf("batch apply failed: %v", err)
    }
    if len(results) != 2 {
        t.Fatalf("unexpected results length: got %d", len(results))
    }

    // Validate on-disk values by direct Get
    valFrom, err := repo.Get(ctx, keyFrom)
    if err != nil {
        t.Fatalf("get from failed: %v", err)
    }
    valTo, err := repo.Get(ctx, keyTo)
    if err != nil {
        t.Fatalf("get to failed: %v", err)
    }

    var bFrom, bTo mmodel.BalanceRedis
    if err := json.Unmarshal([]byte(valFrom), &bFrom); err != nil {
        t.Fatalf("unmarshal from failed: %v", err)
    }
    if err := json.Unmarshal([]byte(valTo), &bTo); err != nil {
        t.Fatalf("unmarshal to failed: %v", err)
    }

    if bFrom.Available.String() != "50" { // 100 - 50
        t.Fatalf("from available mismatch: got %s", bFrom.Available.String())
    }
    if bTo.Available.String() != "50" { // 0 + 50
        t.Fatalf("to available mismatch: got %s", bTo.Available.String())
    }
}

func TestBatchApply_InsufficientFunds_AbortsNoWrites_Valkey(t *testing.T) {
    addr, cleanup := startValkey(t)
    defer cleanup()

    repo := newRedisRepoForAddr(t, addr)
    ctx := context.Background()
    orgID := uuid.New()
    ledgerID := uuid.New()

    keyFrom := libCommons.TransactionInternalKey(orgID, ledgerID, "alias_from")
    keyTo := libCommons.TransactionInternalKey(orgID, ledgerID, "alias_to")

    fromAmount := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(500), Operation: constant.DEBIT}
    toAmount := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(500), Operation: constant.CREDIT}

    // from has only 100 available, non-external
    fromBalance := mmodel.Balance{
        ID:             uuid.New().String(),
        AccountID:      uuid.New().String(),
        Available:      decimal.NewFromInt(100),
        OnHold:         decimal.Zero,
        Version:        1,
        AccountType:    "deposit",
        AllowSending:   true,
        AllowReceiving: true,
        AssetCode:      "USD",
    }
    toBalance := mmodel.Balance{
        ID:             uuid.New().String(),
        AccountID:      uuid.New().String(),
        Available:      decimal.NewFromInt(0),
        OnHold:         decimal.Zero,
        Version:        1,
        AccountType:    "deposit",
        AllowSending:   true,
        AllowReceiving: true,
        AssetCode:      "USD",
    }

    _, err := repo.AddSumBalancesAtomicRedis(ctx, []string{keyFrom, keyTo}, constant.CREATED, false, []libTransaction.Amount{fromAmount, toAmount}, []mmodel.Balance{fromBalance, toBalance})
    if err == nil {
        t.Fatalf("expected error, got nil")
    }

    // Ensure no keys were written
    valFrom, err := repo.Get(ctx, keyFrom)
    if err != nil {
        t.Fatalf("get from failed: %v", err)
    }
    if valFrom != "" {
        t.Fatalf("expected no key for from, got: %s", valFrom)
    }
    valTo, err := repo.Get(ctx, keyTo)
    if err != nil {
        t.Fatalf("get to failed: %v", err)
    }
    if valTo != "" {
        t.Fatalf("expected no key for to, got: %s", valTo)
    }
}

func TestBatchApply_Pending_OnHold_Then_Approved_Debit_Valkey(t *testing.T) {
    addr, cleanup := startValkey(t)
    defer cleanup()

    repo := newRedisRepoForAddr(t, addr)
    ctx := context.Background()
    orgID := uuid.New()
    ledgerID := uuid.New()
    key := libCommons.TransactionInternalKey(orgID, ledgerID, "alias")

    seed := mmodel.Balance{
        ID:             uuid.New().String(),
        AccountID:      uuid.New().String(),
        Available:      decimal.NewFromInt(100),
        OnHold:         decimal.Zero,
        Version:        1,
        AccountType:    "deposit",
        AllowSending:   true,
        AllowReceiving: true,
        AssetCode:      "USD",
    }

    // Step 1: ON_HOLD with PENDING (pending=true, status=PENDING)
    amtHold := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(30), Operation: constant.ONHOLD}
    _, err := repo.AddSumBalancesAtomicRedis(ctx, []string{key}, constant.PENDING, true, []libTransaction.Amount{amtHold}, []mmodel.Balance{seed})
    if err != nil {
        t.Fatalf("on_hold failed: %v", err)
    }

    v1, _ := repo.Get(ctx, key)
    var b1 mmodel.BalanceRedis
    _ = json.Unmarshal([]byte(v1), &b1)
    if b1.Available.String() != "70" || b1.OnHold.String() != "30" {
        t.Fatalf("after on_hold expected avail=70,onHold=30, got avail=%s,onHold=%s", b1.Available, b1.OnHold)
    }

    // Step 2: APPROVED with DEBIT (consume onHold)
    amtApprove := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(30), Operation: constant.DEBIT}
    _, err = repo.AddSumBalancesAtomicRedis(ctx, []string{key}, constant.APPROVED, true, []libTransaction.Amount{amtApprove}, []mmodel.Balance{seed})
    if err != nil {
        t.Fatalf("approved debit failed: %v", err)
    }

    v2, _ := repo.Get(ctx, key)
    var b2 mmodel.BalanceRedis
    _ = json.Unmarshal([]byte(v2), &b2)
    if b2.Available.String() != "70" || b2.OnHold.String() != "0" {
        t.Fatalf("after approved debit expected avail=70,onHold=0, got avail=%s,onHold=%s", b2.Available, b2.OnHold)
    }
}

func TestBatchApply_ExternalPendingFrom_Blocked0098_Valkey(t *testing.T) {
    addr, cleanup := startValkey(t)
    defer cleanup()
    repo := newRedisRepoForAddr(t, addr)
    ctx := context.Background()
    orgID := uuid.New()
    ledgerID := uuid.New()
    key := libCommons.TransactionInternalKey(orgID, ledgerID, "alias")

    seed := mmodel.Balance{
        ID:             uuid.New().String(),
        AccountID:      uuid.New().String(),
        Available:      decimal.Zero,
        OnHold:         decimal.Zero,
        Version:        1,
        AccountType:    "external",
        AllowSending:   true,
        AllowReceiving: true,
        AssetCode:      "USD",
    }

    amtHold := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.ONHOLD}
    _, err := repo.AddSumBalancesAtomicRedis(ctx, []string{key}, constant.PENDING, true, []libTransaction.Amount{amtHold}, []mmodel.Balance{seed})
    var uoe midazpkg.UnprocessableOperationError
    if err == nil || !asUnprocessableWithCode(err, &uoe, constant.ErrOnHoldExternalAccount.Error()) {
        t.Fatalf("expected code %s, got: %v", constant.ErrOnHoldExternalAccount.Error(), err)
    }

    // assert no write
    if val, _ := repo.Get(ctx, key); val != "" {
        t.Fatalf("expected no write for external pending from, got: %s", val)
    }
}

func TestBatchApply_PermissionChecks_0019_Valkey(t *testing.T) {
    addr, cleanup := startValkey(t)
    defer cleanup()
    repo := newRedisRepoForAddr(t, addr)
    ctx := context.Background()
    orgID := uuid.New()
    ledgerID := uuid.New()

    // Debit requires allowSending
    keyDebit := libCommons.TransactionInternalKey(orgID, ledgerID, "alias_debit")
    seedDebit := mmodel.Balance{ID: uuid.New().String(), AccountID: uuid.New().String(), Available: decimal.NewFromInt(100), OnHold: decimal.Zero, Version: 1, AccountType: "deposit", AllowSending: false, AllowReceiving: true, AssetCode: "USD"}
    amtDebit := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.DEBIT}
    _, err := repo.AddSumBalancesAtomicRedis(ctx, []string{keyDebit}, constant.CREATED, false, []libTransaction.Amount{amtDebit}, []mmodel.Balance{seedDebit})
    var uoeD midazpkg.UnprocessableOperationError
    if err == nil || !asUnprocessableWithCode(err, &uoeD, constant.ErrAccountIneligibility.Error()) {
        t.Fatalf("expected code %s for debit, got: %v", constant.ErrAccountIneligibility.Error(), err)
    }
    if val, _ := repo.Get(ctx, keyDebit); val != "" {
        t.Fatalf("expected no write for permission fail, got: %s", val)
    }

    // Credit requires allowReceiving
    keyCredit := libCommons.TransactionInternalKey(orgID, ledgerID, "alias_credit")
    seedCredit := mmodel.Balance{ID: uuid.New().String(), AccountID: uuid.New().String(), Available: decimal.NewFromInt(0), OnHold: decimal.Zero, Version: 1, AccountType: "deposit", AllowSending: true, AllowReceiving: false, AssetCode: "USD"}
    amtCredit := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.CREDIT}
    _, err = repo.AddSumBalancesAtomicRedis(ctx, []string{keyCredit}, constant.CREATED, false, []libTransaction.Amount{amtCredit}, []mmodel.Balance{seedCredit})
    var uoeC midazpkg.UnprocessableOperationError
    if err == nil || !asUnprocessableWithCode(err, &uoeC, constant.ErrAccountIneligibility.Error()) {
        t.Fatalf("expected code %s for credit, got: %v", constant.ErrAccountIneligibility.Error(), err)
    }
    if val, _ := repo.Get(ctx, keyCredit); val != "" {
        t.Fatalf("expected no write for permission fail, got: %s", val)
    }
}

func TestBatchApply_TTL_Preservation_Valkey(t *testing.T) {
    addr, cleanup := startValkey(t)
    defer cleanup()
    repo := newRedisRepoForAddr(t, addr)
    ctx := context.Background()
    orgID := uuid.New()
    ledgerID := uuid.New()
    key := libCommons.TransactionInternalKey(orgID, ledgerID, "alias")

    // Seed key manually with TTL=120s
    seedRedis := map[string]any{
        "id":         uuid.New().String(),
        "accountId":  uuid.New().String(),
        "assetCode":  "USD",
        "available":  100,
        "onHold":     0,
        "version":    1,
        "accountType": "deposit",
        "allowSending": 1,
        "allowReceiving": 1,
    }
    raw, _ := json.Marshal(seedRedis)
    // Note: Set expects TTL in seconds, not duration; pass 120 directly
    if err := repo.Set(ctx, key, string(raw), time.Duration(120)); err != nil {
        t.Fatalf("seed set with ttl failed: %v", err)
    }

    // Fetch TTL before update
    client, err := repo.conn.GetClient(ctx)
    if err != nil {
        t.Fatalf("get client failed: %v", err)
    }
    ttlBefore, err := client.TTL(ctx, key).Result()
    if err != nil {
        t.Fatalf("ttl before failed: %v", err)
    }
    if ttlBefore <= 0 {
        t.Fatalf("expected positive ttl before, got %v", ttlBefore)
    }

    // Apply a credit update via batch
    seed := mmodel.Balance{ID: seedRedis["id"].(string), AccountID: seedRedis["accountId"].(string), Available: decimal.NewFromInt(100), OnHold: decimal.Zero, Version: 1, AccountType: "deposit", AllowSending: true, AllowReceiving: true, AssetCode: "USD"}
    amt := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.CREDIT}
    if _, err := repo.AddSumBalancesAtomicRedis(ctx, []string{key}, constant.CREATED, false, []libTransaction.Amount{amt}, []mmodel.Balance{seed}); err != nil {
        t.Fatalf("batch credit failed: %v", err)
    }

    ttlAfter, err := client.TTL(ctx, key).Result()
    if err != nil {
        t.Fatalf("ttl after failed: %v", err)
    }
    if ttlAfter <= 0 || ttlAfter > ttlBefore {
        t.Fatalf("ttl not preserved/reasonable: before=%v after=%v", ttlBefore, ttlAfter)
    }
}

func TestBatchApply_OccVersionMismatch_0086_Valkey(t *testing.T) {
    addr, cleanup := startValkey(t)
    defer cleanup()
    repo := newRedisRepoForAddr(t, addr)
    ctx := context.Background()
    orgID := uuid.New()
    ledgerID := uuid.New()
    key := libCommons.TransactionInternalKey(orgID, ledgerID, "alias")

    // Seed key with specific version 5
    seed := mmodel.Balance{ID: uuid.New().String(), AccountID: uuid.New().String(), Available: decimal.NewFromInt(100), OnHold: decimal.Zero, Version: 5, AccountType: "deposit", AllowSending: true, AllowReceiving: true, AssetCode: "USD"}
    // Write canonical JSON directly
    b := map[string]any{
        "id": seed.ID, "accountId": seed.AccountID, "assetCode": seed.AssetCode,
        "available": 100, "onHold": 0, "version": 5, "accountType": "deposit", "allowSending": 1, "allowReceiving": 1,
    }
    raw, _ := json.Marshal(b)
    if err := repo.Set(ctx, key, string(raw), 0); err != nil {
        t.Fatalf("seed set failed: %v", err)
    }

    // Directly invoke the Lua with enforceOCC=1 and mismatching provided version (e.g., 3)
    client, err := repo.conn.GetClient(ctx)
    if err != nil {
        t.Fatalf("get client failed: %v", err)
    }

    // Build keys and args per script contract
    keys := []string{key}
    isPending := 0
    transactionStatus := constant.CREATED
    enforceOCC := 1
    amt := libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.CREDIT}
    args := []any{isPending, transactionStatus, enforceOCC,
        amt.Operation,
        amt.Value.String(),
        seed.ID,
        seed.Available.String(),
        seed.OnHold.String(),
        "3", // provided wrong version (mismatch with stored 5)
        seed.AccountType,
        1, 1,
        seed.AssetCode,
        seed.AccountID,
    }

    script := rds.NewScript(batchApplyLua)
    _, err = script.Run(ctx, client, keys, args...).Result()
    if err == nil || !strings.Contains(err.Error(), constant.ErrLockVersionAccountBalance.Error()) {
        t.Fatalf("expected 0086 error on OCC mismatch, got: %v", err)
    }
}

// asUnprocessableWithCode helper checks if error is UnprocessableOperationError with a specific business code
func asUnprocessableWithCode(err error, target *midazpkg.UnprocessableOperationError, code string) bool {
    if err == nil {
        return false
    }
    if !errors.As(err, target) {
        return false
    }
    return target.Code == code
}
