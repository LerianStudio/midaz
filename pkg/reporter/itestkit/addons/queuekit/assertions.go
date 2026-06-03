package queuekit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// Assertions provides test assertion helpers for messages.
type Assertions[T any] struct {
	t       *testing.T
	message ParsedMessage[T]
}

// AssertMessage creates an Assertions helper for a parsed message.
func AssertMessage[T any](t *testing.T, msg ParsedMessage[T]) *Assertions[T] {
	t.Helper()

	return &Assertions[T]{
		t:       t,
		message: msg,
	}
}

// HasRoutingKey asserts the message has the expected routing key.
func (a *Assertions[T]) HasRoutingKey(expected string) *Assertions[T] {
	a.t.Helper()

	if a.message.RoutingKey != expected {
		a.t.Errorf("expected routing key %q, got %q", expected, a.message.RoutingKey)
	}

	return a
}

// HasHeader asserts the message has a header with the expected value.
func (a *Assertions[T]) HasHeader(key string, expected any) *Assertions[T] {
	a.t.Helper()

	actual, ok := a.message.Headers[key]
	if !ok {
		a.t.Errorf("expected header %q to exist", key)
		return a
	}

	if !reflect.DeepEqual(actual, expected) {
		a.t.Errorf("expected header %q to be %v, got %v", key, expected, actual)
	}

	return a
}

// HasHeaderKey asserts the message has the specified header key.
func (a *Assertions[T]) HasHeaderKey(key string) *Assertions[T] {
	a.t.Helper()

	if _, ok := a.message.Headers[key]; !ok {
		a.t.Errorf("expected header %q to exist", key)
	}

	return a
}

// HasCorrelationID asserts the message has the expected correlation ID.
func (a *Assertions[T]) HasCorrelationID(expected string) *Assertions[T] {
	a.t.Helper()

	if a.message.CorrelationID != expected {
		a.t.Errorf("expected correlation ID %q, got %q", expected, a.message.CorrelationID)
	}

	return a
}

// HasMessageID asserts the message has the expected message ID.
func (a *Assertions[T]) HasMessageID(expected string) *Assertions[T] {
	a.t.Helper()

	if a.message.MessageID != expected {
		a.t.Errorf("expected message ID %q, got %q", expected, a.message.MessageID)
	}

	return a
}

// HasContentType asserts the message has the expected content type.
func (a *Assertions[T]) HasContentType(expected string) *Assertions[T] {
	a.t.Helper()

	if a.message.ContentType != expected {
		a.t.Errorf("expected content type %q, got %q", expected, a.message.ContentType)
	}

	return a
}

// PayloadEquals asserts the payload equals the expected value.
func (a *Assertions[T]) PayloadEquals(expected T) *Assertions[T] {
	a.t.Helper()

	if !reflect.DeepEqual(a.message.Payload, expected) {
		a.t.Errorf("payload mismatch:\nexpected: %+v\ngot: %+v", expected, a.message.Payload)
	}

	return a
}

// PayloadSatisfies asserts the payload satisfies a custom predicate.
func (a *Assertions[T]) PayloadSatisfies(name string, predicate func(T) bool) *Assertions[T] {
	a.t.Helper()

	if !predicate(a.message.Payload) {
		a.t.Errorf("payload does not satisfy predicate %q: %+v", name, a.message.Payload)
	}

	return a
}

// Payload returns the payload for further assertions.
func (a *Assertions[T]) Payload() T {
	return a.message.Payload
}

// Message returns the original message.
func (a *Assertions[T]) Message() ParsedMessage[T] {
	return a.message
}

// ResultAssertions provides assertions for WaitResult.
type ResultAssertions[T any] struct {
	t      *testing.T
	result WaitResult[T]
}

// AssertResult creates assertions for a WaitResult.
func AssertResult[T any](t *testing.T, result WaitResult[T]) *ResultAssertions[T] {
	t.Helper()

	return &ResultAssertions[T]{
		t:      t,
		result: result,
	}
}

// HasCount asserts the result has exactly n matched messages.
func (a *ResultAssertions[T]) HasCount(n int) *ResultAssertions[T] {
	a.t.Helper()

	if len(a.result.Messages) != n {
		a.t.Errorf("expected %d messages, got %d", n, len(a.result.Messages))
	}

	return a
}

// HasAtLeast asserts the result has at least n matched messages.
func (a *ResultAssertions[T]) HasAtLeast(n int) *ResultAssertions[T] {
	a.t.Helper()

	if len(a.result.Messages) < n {
		a.t.Errorf("expected at least %d messages, got %d", n, len(a.result.Messages))
	}

	return a
}

// HasNoErrors asserts no parsing errors occurred.
func (a *ResultAssertions[T]) HasNoErrors() *ResultAssertions[T] {
	a.t.Helper()

	if len(a.result.Errors) > 0 {
		a.t.Errorf("expected no errors, got %d: %v", len(a.result.Errors), a.result.Errors)
	}

	return a
}

// DidNotTimeout asserts the wait did not timeout.
func (a *ResultAssertions[T]) DidNotTimeout() *ResultAssertions[T] {
	a.t.Helper()

	if a.result.TimedOut {
		a.t.Errorf("wait timed out after %v", a.result.Duration)
	}

	return a
}

// UnmatchedCount asserts the number of unmatched messages.
func (a *ResultAssertions[T]) UnmatchedCount(n int) *ResultAssertions[T] {
	a.t.Helper()

	if len(a.result.Unmatched) != n {
		a.t.Errorf("expected %d unmatched messages, got %d", n, len(a.result.Unmatched))
	}

	return a
}

// First returns assertions for the first message.
func (a *ResultAssertions[T]) First() *Assertions[T] {
	a.t.Helper()

	if len(a.result.Messages) == 0 {
		a.t.Fatal("no messages to assert on")
		return nil // unreachable
	}

	return AssertMessage(a.t, a.result.Messages[0])
}

// At returns assertions for the message at index i.
func (a *ResultAssertions[T]) At(i int) *Assertions[T] {
	a.t.Helper()

	if i < 0 || i >= len(a.result.Messages) {
		a.t.Fatalf("index %d out of range (have %d messages)", i, len(a.result.Messages))
		return nil // unreachable
	}

	return AssertMessage(a.t, a.result.Messages[i])
}

// All applies a function to all messages for custom assertions.
func (a *ResultAssertions[T]) All(fn func(t *testing.T, i int, msg ParsedMessage[T])) *ResultAssertions[T] {
	a.t.Helper()

	for i, msg := range a.result.Messages {
		fn(a.t, i, msg)
	}

	return a
}

// Messages returns all matched messages.
func (a *ResultAssertions[T]) Messages() []ParsedMessage[T] {
	return a.result.Messages
}

// Result returns the original result.
func (a *ResultAssertions[T]) Result() WaitResult[T] {
	return a.result
}

// JSONEqual checks if two JSON values are equal (ignoring formatting).
func JSONEqual(t *testing.T, expected, actual []byte) bool {
	t.Helper()

	var e, a any
	if err := json.Unmarshal(expected, &e); err != nil {
		t.Errorf("failed to unmarshal expected JSON: %v", err)
		return false
	}

	if err := json.Unmarshal(actual, &a); err != nil {
		t.Errorf("failed to unmarshal actual JSON: %v", err)
		return false
	}

	return reflect.DeepEqual(e, a)
}

// AssertJSONEqual asserts two JSON values are equal.
func AssertJSONEqual(t *testing.T, expected, actual []byte) {
	t.Helper()

	if !JSONEqual(t, expected, actual) {
		t.Errorf("JSON mismatch:\nexpected: %s\nactual: %s", expected, actual)
	}
}

// AssertJSONField asserts a JSON field has the expected value.
func AssertJSONField(t *testing.T, data []byte, path string, expected any) {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Errorf("failed to unmarshal JSON: %v", err)
		return
	}

	actual := getNestedValue(m, path)
	if !compareValues(actual, expected) {
		t.Errorf("field %q: expected %v (%T), got %v (%T)", path, expected, expected, actual, actual)
	}
}

// MessageSequence helps verify message ordering.
type MessageSequence[T any] struct {
	messages []ParsedMessage[T]
}

// NewSequence creates a sequence from messages.
func NewSequence[T any](messages []ParsedMessage[T]) *MessageSequence[T] {
	return &MessageSequence[T]{messages: messages}
}

// RoutingKeysInOrder returns the routing keys in order.
func (s *MessageSequence[T]) RoutingKeysInOrder() []string {
	keys := make([]string, len(s.messages))
	for i, m := range s.messages {
		keys[i] = m.RoutingKey
	}

	return keys
}

// AssertOrder verifies messages came in the expected order based on a key extractor.
func (s *MessageSequence[T]) AssertOrder(t *testing.T, keyFn func(T) string, expectedOrder []string) {
	t.Helper()

	if len(s.messages) != len(expectedOrder) {
		t.Errorf("expected %d messages, got %d", len(expectedOrder), len(s.messages))
		return
	}

	for i, msg := range s.messages {
		actual := keyFn(msg.Payload)
		if actual != expectedOrder[i] {
			t.Errorf("message %d: expected key %q, got %q", i, expectedOrder[i], actual)
		}
	}
}

// FilterBy filters messages by a predicate.
func (s *MessageSequence[T]) FilterBy(predicate func(T) bool) []ParsedMessage[T] {
	var result []ParsedMessage[T]

	for _, m := range s.messages {
		if predicate(m.Payload) {
			result = append(result, m)
		}
	}

	return result
}

// GroupBy groups messages by a key function.
func (s *MessageSequence[T]) GroupBy(keyFn func(T) string) map[string][]ParsedMessage[T] {
	groups := make(map[string][]ParsedMessage[T])

	for _, m := range s.messages {
		key := keyFn(m.Payload)
		groups[key] = append(groups[key], m)
	}

	return groups
}

// ExpectMessages is a convenience wrapper for common assertion patterns.
func ExpectMessages[T any](t *testing.T, result WaitResult[T]) *ExpectMessagesHelper[T] {
	t.Helper()
	return &ExpectMessagesHelper[T]{t: t, result: result}
}

// ExpectMessagesHelper provides chainable expectations.
type ExpectMessagesHelper[T any] struct {
	t      *testing.T
	result WaitResult[T]
	failed bool
}

// ToSucceed asserts the wait completed without timeout or errors.
func (e *ExpectMessagesHelper[T]) ToSucceed() *ExpectMessagesHelper[T] {
	e.t.Helper()

	if e.result.TimedOut {
		e.t.Errorf("expected success but timed out after %v", e.result.Duration)
		e.failed = true
	}

	if len(e.result.Errors) > 0 {
		e.t.Errorf("expected no errors but got %d: %v", len(e.result.Errors), e.result.Errors)
		e.failed = true
	}

	return e
}

// ToHaveCount asserts the message count.
func (e *ExpectMessagesHelper[T]) ToHaveCount(n int) *ExpectMessagesHelper[T] {
	e.t.Helper()

	if len(e.result.Messages) != n {
		e.t.Errorf("expected %d messages, got %d", n, len(e.result.Messages))
		e.failed = true
	}

	return e
}

// ToContainWhere asserts at least one message matches a predicate.
func (e *ExpectMessagesHelper[T]) ToContainWhere(name string, predicate func(T) bool) *ExpectMessagesHelper[T] {
	e.t.Helper()

	for _, m := range e.result.Messages {
		if predicate(m.Payload) {
			return e
		}
	}

	e.t.Errorf("expected to contain message matching %q", name)
	e.failed = true

	return e
}

// Failed returns true if any assertion failed.
func (e *ExpectMessagesHelper[T]) Failed() bool {
	return e.failed
}

// OrFatal fails the test immediately if any assertion failed.
func (e *ExpectMessagesHelper[T]) OrFatal() {
	e.t.Helper()

	if e.failed {
		e.t.FailNow()
	}
}

// Summary returns a human-readable summary of the result.
func Summary[T any](result WaitResult[T]) string {
	return fmt.Sprintf(
		"matched=%d unmatched=%d errors=%d duration=%v timedOut=%v",
		len(result.Messages),
		len(result.Unmatched),
		len(result.Errors),
		result.Duration,
		result.TimedOut,
	)
}
