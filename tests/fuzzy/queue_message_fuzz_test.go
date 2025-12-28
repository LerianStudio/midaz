package fuzzy

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	fuzz "github.com/google/gofuzz"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// FuzzQueueJSONUnmarshal tests JSON parsing of queue messages with gofuzz-generated
// diverse Queue structs plus manual edge cases.
// Simulates handlerBalanceCreateQueue at rabbitmq.server.go:118.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzQueueJSONUnmarshal -run=^$ -fuzztime=60s
func FuzzQueueJSONUnmarshal(f *testing.F) {
	// Use gofuzz to generate diverse Queue structs then serialize to JSON
	fuzzer := fuzz.New().NilChance(0.1).NumElements(0, 10).Funcs(
		// Custom fuzzer for uuid.UUID
		func(u *uuid.UUID, c fuzz.Continue) {
			choices := []uuid.UUID{
				uuid.MustParse("00000000-0000-0000-0000-000000000000"),
				uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
				uuid.New(),
			}
			*u = choices[c.Intn(len(choices))]
		},
		// Custom fuzzer for json.RawMessage
		func(r *json.RawMessage, c fuzz.Continue) {
			choices := []json.RawMessage{
				json.RawMessage(`{}`),
				json.RawMessage(`{"amount":1000}`),
				json.RawMessage(`{"type":"balance","value":500}`),
				json.RawMessage(`null`),
				json.RawMessage(`[]`),
			}
			*r = choices[c.Intn(len(choices))]
		},
	)

	// Generate 20 diverse JSON seeds using gofuzz
	for i := 0; i < 20; i++ {
		var q mmodel.Queue
		fuzzer.Fuzz(&q)
		// Serialize to JSON
		jsonBytes, err := json.Marshal(q)
		if err == nil {
			f.Add(string(jsonBytes))
		}
	}

	// Manual seeds: valid queue messages
	f.Add(`{"organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","auditId":"00000000-0000-0000-0000-000000000003","accountId":"00000000-0000-0000-0000-000000000004","queueData":[]}`)

	// With queue data
	f.Add(`{"organizationId":"550e8400-e29b-41d4-a716-446655440000","ledgerId":"550e8400-e29b-41d4-a716-446655440001","auditId":"550e8400-e29b-41d4-a716-446655440002","accountId":"550e8400-e29b-41d4-a716-446655440003","queueData":[{"id":"550e8400-e29b-41d4-a716-446655440004","value":{"amount":1000}}]}`)

	// Empty object
	f.Add(`{}`)

	// Invalid UUIDs
	f.Add(`{"organizationId":"not-a-uuid","ledgerId":"also-not-uuid"}`)
	f.Add(`{"organizationId":"","ledgerId":""}`)

	// Manual seeds: malformed JSON
	f.Add(`{"organizationId":`)
	f.Add(`{"organizationId":"uuid"`)
	f.Add(``)
	f.Add(`null`)
	f.Add(`[]`)
	f.Add(`"string"`)

	// Manual seeds: large payloads
	largeValue := `{"organizationId":"550e8400-e29b-41d4-a716-446655440000","queueData":[`
	for i := 0; i < 100; i++ {
		if i > 0 {
			largeValue += ","
		}
		largeValue += `{"id":"550e8400-e29b-41d4-a716-446655440000","value":{}}`
	}
	largeValue += `]}`
	f.Add(largeValue)

	// Manual seeds: injection attempts
	f.Add(`{"organizationId":"'; DROP TABLE--","queueData":[]}`)
	f.Add(`{"organizationId":"{{.Exec}}","queueData":[]}`)

	// Manual seeds: binary data in strings
	f.Add(`{"organizationId":"\x00\x01\x02","queueData":[]}`)

	// Manual seeds: deeply nested
	f.Add(`{"queueData":[{"value":{"nested":{"deep":{"very":{"deep":{"data":"test"}}}}}}]}`)

	f.Fuzz(func(t *testing.T, jsonData string) {
		// Skip invalid UTF-8
		if !utf8.ValidString(jsonData) {
			return
		}

		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("json.Unmarshal(Queue) panicked: %v\nInput: %q",
					r, truncateStringQueue(jsonData, 200))
			}
		}()

		var message mmodel.Queue
		err := json.Unmarshal([]byte(jsonData), &message)

		// If successful, access fields to ensure they're valid
		if err == nil {
			_ = message.OrganizationID.String()
			_ = message.LedgerID.String()
			_ = message.AuditID.String()
			_ = message.AccountID.String()
			_ = len(message.QueueData)
		}
	})
}

// FuzzQueueMsgpackUnmarshal tests msgpack parsing of queue messages with gofuzz-generated
// diverse Queue structs serialized to msgpack plus manual edge cases.
// Simulates handlerBTOQueue at rabbitmq.server.go:158.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzQueueMsgpackUnmarshal -run=^$ -fuzztime=60s
func FuzzQueueMsgpackUnmarshal(f *testing.F) {
	// Use gofuzz to generate diverse Queue structs then serialize to msgpack
	fuzzer := fuzz.New().NilChance(0.1).NumElements(0, 10).Funcs(
		// Custom fuzzer for uuid.UUID
		func(u *uuid.UUID, c fuzz.Continue) {
			choices := []uuid.UUID{
				uuid.MustParse("00000000-0000-0000-0000-000000000000"),
				uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
				uuid.New(),
			}
			*u = choices[c.Intn(len(choices))]
		},
		// Custom fuzzer for json.RawMessage
		func(r *json.RawMessage, c fuzz.Continue) {
			choices := []json.RawMessage{
				json.RawMessage(`{}`),
				json.RawMessage(`{"amount":1000}`),
				json.RawMessage(`null`),
			}
			*r = choices[c.Intn(len(choices))]
		},
	)

	// Generate 20 diverse msgpack seeds using gofuzz
	for i := 0; i < 20; i++ {
		var q mmodel.Queue
		fuzzer.Fuzz(&q)
		// Serialize to msgpack
		msgpackBytes, err := msgpack.Marshal(q)
		if err == nil {
			f.Add(msgpackBytes)
		}
	}

	// Manual seeds: valid msgpack-encoded queue messages
	validQueue := mmodel.Queue{}
	validBytes, _ := msgpack.Marshal(validQueue)
	f.Add(validBytes)

	// Manual seeds: empty and minimal
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x80}) // empty map in msgpack
	f.Add([]byte{0x90}) // empty array in msgpack
	f.Add([]byte{0xc0}) // nil in msgpack

	// Manual seeds: random binary patterns
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0xde, 0xad, 0xbe, 0xef})
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05})

	// Manual seeds: truncated msgpack
	f.Add([]byte{0x85}) // map with 5 elements but no data

	// Manual seeds: large payload
	largePayload := make([]byte, 10000)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}
	f.Add(largePayload)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("msgpack.Unmarshal(Queue) panicked: %v\nInput len=%d, first bytes=%x",
					r, len(data), truncateBytesQueue(data, 50))
			}
		}()

		var message mmodel.Queue
		err := msgpack.Unmarshal(data, &message)

		// If successful, access fields to ensure they're valid
		if err == nil {
			_ = message.OrganizationID.String()
			_ = message.LedgerID.String()
			_ = len(message.QueueData)
		}
	})
}

// truncateStringQueue safely truncates a string to maxLen characters
func truncateStringQueue(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// truncateBytesQueue returns first n bytes or all if shorter
func truncateBytesQueue(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}
