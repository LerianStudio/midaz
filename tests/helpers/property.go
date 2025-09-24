package helpers

import (
    "math/rand"
    "os"
    "strconv"
    "testing"
    "testing/quick"
    "time"
)

// propEnv fetches an environment variable with a default fallback.
func propEnv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

// quickConfigFromEnv builds a testing/quick.Config using environment variables:
//  - MIDAZ_PROP_MAXCOUNT (int, default 100)
//  - MIDAZ_PROP_SCALE (float64, default 1.0)
//  - MIDAZ_PROP_SEED (int64, default now)
func quickConfigFromEnv(t *testing.T) *quick.Config {
    maxCountStr := propEnv("MIDAZ_PROP_MAXCOUNT", "100")
    maxCount, err := strconv.Atoi(maxCountStr)
    if err != nil || maxCount <= 0 {
        maxCount = 100
    }

    scaleStr := propEnv("MIDAZ_PROP_SCALE", "1.0")
    scale, err := strconv.ParseFloat(scaleStr, 64)
    if err != nil || scale <= 0 {
        scale = 1.0
    }

    var seed int64
    if v := os.Getenv("MIDAZ_PROP_SEED"); v != "" {
        if s, err := strconv.ParseInt(v, 10, 64); err == nil {
            seed = s
        }
    }
    if seed == 0 {
        seed = time.Now().UnixNano()
    }
    t.Logf("property seed=%d maxCount=%d scale=%.2f", seed, maxCount, scale)

    return &quick.Config{
        MaxCount:      maxCount,
        MaxCountScale: scale,
        Rand:          rand.New(rand.NewSource(seed)),
    }
}

// CheckProp runs testing/quick.Check with a config derived from environment.
// It fails the test with a helpful message on the first counterexample.
func CheckProp(t *testing.T, f any) {
    t.Helper()
    cfg := quickConfigFromEnv(t)
    if err := quick.Check(f, cfg); err != nil {
        t.Fatalf("property failed: %v", err)
    }
}

// CheckPropEqual runs testing/quick.CheckEqual using the env-derived config.
func CheckPropEqual(t *testing.T, f, g any) {
    t.Helper()
    cfg := quickConfigFromEnv(t)
    if err := quick.CheckEqual(f, g, cfg); err != nil {
        t.Fatalf("property equality failed: %v", err)
    }
}

