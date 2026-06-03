package queuekit

import (
	"context"
	"testing"
	"time"
)

// mockConsumer is a test double for QueueConsumer.
type mockConsumer struct {
	messages []Message
	err      error
	closed   bool
}

func newMockConsumer(messages []Message) *mockConsumer {
	return &mockConsumer{messages: messages}
}

func (m *mockConsumer) Consume(ctx context.Context) (<-chan Message, error) {
	if m.err != nil {
		return nil, m.err
	}

	ch := make(chan Message)
	go func() {
		defer close(ch)
		for _, msg := range m.messages {
			select {
			case <-ctx.Done():
				return
			case ch <- msg:
			}
		}
	}()
	return ch, nil
}

func (m *mockConsumer) Close() error {
	m.closed = true
	return nil
}

// TestPayload is a test struct for unmarshaling.
type TestPayload struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
}

func TestConsumerWaitForMessage(t *testing.T) {
	messages := []Message{
		{Body: []byte(`{"jobId": "123", "status": "completed"}`), RoutingKey: "job.completed"},
	}

	backend := newMockConsumer(messages)
	consumer := NewConsumer[TestPayload](t, backend).
		WithTimeout(5 * time.Second).
		Build()
	defer consumer.Close()

	msg, err := consumer.WaitForMessage(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Payload.JobID != "123" {
		t.Errorf("expected jobId 123, got %s", msg.Payload.JobID)
	}
	if msg.Payload.Status != "completed" {
		t.Errorf("expected status completed, got %s", msg.Payload.Status)
	}
}

func TestConsumerWaitForMessageWithMatcher(t *testing.T) {
	messages := []Message{
		{Body: []byte(`{"jobId": "111", "status": "pending"}`), RoutingKey: "job.pending"},
		{Body: []byte(`{"jobId": "222", "status": "completed"}`), RoutingKey: "job.completed"},
		{Body: []byte(`{"jobId": "333", "status": "completed"}`), RoutingKey: "job.completed"},
	}

	backend := newMockConsumer(messages)
	consumer := NewConsumer[TestPayload](t, backend).
		WithTimeout(5 * time.Second).
		WithMatcher(MatchJSONField("jobId", "222")).
		Build()
	defer consumer.Close()

	msg, err := consumer.WaitForMessage(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Payload.JobID != "222" {
		t.Errorf("expected jobId 222, got %s", msg.Payload.JobID)
	}
}

func TestConsumerWaitForMessages(t *testing.T) {
	messages := []Message{
		{Body: []byte(`{"jobId": "1", "status": "completed"}`), RoutingKey: "job.completed"},
		{Body: []byte(`{"jobId": "2", "status": "completed"}`), RoutingKey: "job.completed"},
		{Body: []byte(`{"jobId": "3", "status": "completed"}`), RoutingKey: "job.completed"},
	}

	backend := newMockConsumer(messages)
	consumer := NewConsumer[TestPayload](t, backend).
		WithTimeout(5 * time.Second).
		Build()
	defer consumer.Close()

	result, err := consumer.WaitForMessages(context.Background(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result.Messages))
	}
}

func TestConsumerWaitForMessagesTimeout(t *testing.T) {
	messages := []Message{
		{Body: []byte(`{"jobId": "1", "status": "completed"}`)},
	}

	backend := newMockConsumer(messages)
	consumer := NewConsumer[TestPayload](t, backend).
		WithTimeout(100 * time.Millisecond).
		Build()
	defer consumer.Close()

	_, err := consumer.WaitForMessages(context.Background(), 5)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestConsumerCaptureAll(t *testing.T) {
	messages := []Message{
		{Body: []byte(`{"jobId": "1", "status": "completed"}`)},
		{Body: []byte(`{"jobId": "2", "status": "failed"}`)},
		{Body: []byte(`{"jobId": "3", "status": "completed"}`)},
	}

	backend := newMockConsumer(messages)
	consumer := NewConsumer[TestPayload](t, backend).
		WithMatcher(MatchJSONField("status", "completed")).
		Build()
	defer consumer.Close()

	result, err := consumer.CaptureAll(context.Background(), 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Messages) != 2 {
		t.Errorf("expected 2 matched messages, got %d", len(result.Messages))
	}

	if len(result.Unmatched) != 1 {
		t.Errorf("expected 1 unmatched message, got %d", len(result.Unmatched))
	}
}

func TestConsumerAssertNoMessages(t *testing.T) {
	backend := newMockConsumer([]Message{})
	consumer := NewConsumer[TestPayload](t, backend).Build()
	defer consumer.Close()

	err := consumer.AssertNoMessages(context.Background(), 100*time.Millisecond)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestConsumerAssertNoMessagesWithMessages(t *testing.T) {
	messages := []Message{
		{Body: []byte(`{"jobId": "1", "status": "completed"}`)},
	}

	backend := newMockConsumer(messages)
	consumer := NewConsumer[TestPayload](t, backend).Build()
	defer consumer.Close()

	err := consumer.AssertNoMessages(context.Background(), 100*time.Millisecond)
	if err == nil {
		t.Error("expected error when messages present")
	}
}

func TestConsumerDrainQueue(t *testing.T) {
	messages := []Message{
		{Body: []byte(`{"jobId": "1"}`)},
		{Body: []byte(`{"jobId": "2"}`)},
		{Body: []byte(`{"jobId": "3"}`)},
	}

	backend := newMockConsumer(messages)
	consumer := NewConsumer[TestPayload](t, backend).Build()
	defer consumer.Close()

	count, err := consumer.DrainQueue(context.Background(), 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 drained messages, got %d", count)
	}
}

func TestWaitForConvenience(t *testing.T) {
	messages := []Message{
		{Body: []byte(`{"jobId": "123", "status": "completed"}`)},
	}

	backend := newMockConsumer(messages)

	msg, err := WaitFor[TestPayload](
		context.Background(),
		t,
		backend,
		MatchAlways(),
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Payload.JobID != "123" {
		t.Errorf("expected jobId 123, got %s", msg.Payload.JobID)
	}
}

func TestWaitResult(t *testing.T) {
	result := WaitResult[TestPayload]{
		Messages: []ParsedMessage[TestPayload]{
			{Payload: TestPayload{JobID: "1"}},
			{Payload: TestPayload{JobID: "2"}},
		},
	}

	if result.Count() != 2 {
		t.Errorf("expected count 2, got %d", result.Count())
	}

	first, ok := result.First()
	if !ok {
		t.Error("expected First() to return true")
	}
	if first.Payload.JobID != "1" {
		t.Errorf("expected first jobId 1, got %s", first.Payload.JobID)
	}
}

func TestWaitResultEmpty(t *testing.T) {
	result := WaitResult[TestPayload]{}

	if result.Count() != 0 {
		t.Errorf("expected count 0, got %d", result.Count())
	}

	_, ok := result.First()
	if ok {
		t.Error("expected First() to return false for empty result")
	}
}
