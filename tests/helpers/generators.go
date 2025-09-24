package helpers

import (
    "fmt"
    "math/rand"
    "time"
)

// Initialize package-local RNG for generators.
var genRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// RandomAlias returns a short unique alias with an optional prefix.
func RandomAlias(prefix string) string {
    if prefix == "" { prefix = "a" }
    return fmt.Sprintf("%s-%s", prefix, RandString(6))
}

// RandomCode returns an uppercase code-like value with digits.
func RandomCode(prefix string, n int) string {
    if prefix == "" { prefix = "C" }
    letters := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
    b := make([]rune, n)
    for i := range b { b[i] = letters[genRand.Intn(len(letters))] }
    return fmt.Sprintf("%s-%s", prefix, string(b))
}

// OrgPayloadRandom constructs a minimal valid organization payload.
func OrgPayloadRandom() map[string]any {
    return OrgPayload(fmt.Sprintf("Org %s", RandString(6)), RandString(14))
}

// LedgerPayloadRandom constructs a minimal valid ledger payload.
func LedgerPayloadRandom() map[string]any {
    return map[string]any{
        "name": fmt.Sprintf("Ledger %s", RandString(5)),
        // Optionally, add code metadata later
    }
}

// AccountPayloadRandom returns a minimal valid account payload for a given asset code and type.
// Common types: deposit, liability, revenue, expense, equity
func AccountPayloadRandom(assetCode, typ, aliasPrefix string) map[string]any {
    if assetCode == "" { assetCode = "USD" }
    if typ == "" { typ = "deposit" }
    alias := RandomAlias(aliasPrefix)
    return map[string]any{
        "name":      fmt.Sprintf("Acc %s", RandString(4)),
        "assetCode": assetCode,
        "type":      typ,
        "alias":     alias,
    }
}

// AssetPayload returns a minimal asset payload for a fiat currency (e.g., USD).
func AssetPayload(code, name string) map[string]any {
    if code == "" { code = "USD" }
    if name == "" { name = "US Dollar" }
    return map[string]any{
        "name": name,
        "type": "currency",
        "code": code,
    }
}

// InflowPayload builds a minimal inflow transaction distributing amount to a single alias.
// value must be a decimal string (e.g., "12.50").
func InflowPayload(asset, value, accountAlias string) map[string]any {
    if asset == "" { asset = "USD" }
    return map[string]any{
        "send": map[string]any{
            "asset": asset,
            "value": value,
            "distribute": map[string]any{
                "to": []map[string]any{
                    {
                        "accountAlias": accountAlias,
                        "amount": map[string]any{"asset": asset, "value": value},
                    },
                },
            },
        },
    }
}

// OutflowPayload builds a minimal outflow transaction sourcing amount from a single alias.
func OutflowPayload(pending bool, asset, value, accountAlias string) map[string]any {
    if asset == "" { asset = "USD" }
    return map[string]any{
        "pending": pending,
        "send": map[string]any{
            "asset": asset,
            "value": value,
            "source": map[string]any{
                "from": []map[string]any{
                    {
                        "accountAlias": accountAlias,
                        "amount": map[string]any{"asset": asset, "value": value},
                    },
                },
            },
        },
    }
}

