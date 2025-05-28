package saga

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg/events"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// Status represents the status of a saga
type Status string

const (
	StatusPending      Status = "pending"
	StatusRunning      Status = "running"
	StatusCompleted    Status = "completed"
	StatusFailed       Status = "failed"
	StatusCompensating Status = "compensating"
	StatusCompensated  Status = "compensated"
	StatusAborted      Status = "aborted"
)

// StepStatus represents the status of a saga step
type StepStatus string

const (
	StepStatusPending     StepStatus = "pending"
	StepStatusRunning     StepStatus = "running"
	StepStatusCompleted   StepStatus = "completed"
	StepStatusFailed      StepStatus = "failed"
	StepStatusCompensated StepStatus = "compensated"
	StepStatusSkipped     StepStatus = "skipped"
)

// Step represents a single step in a saga
type Step struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Status        StepStatus             `json:"status"`
	StartedAt     *time.Time             `json:"started_at,omitempty"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	Error         *string                `json:"error,omitempty"`
	CompensatedAt *time.Time             `json:"compensated_at,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	RetryCount    int                    `json:"retry_count"`
	MaxRetries    int                    `json:"max_retries"`
}

// Saga represents a distributed transaction
type Saga struct {
	ID               uuid.UUID              `json:"id"`
	Name             string                 `json:"name"`
	Status           Status                 `json:"status"`
	Steps            []Step                 `json:"steps"`
	CurrentStepIndex int                    `json:"current_step_index"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	OrganizationID   uuid.UUID              `json:"organization_id"`
	CorrelationID    string                 `json:"correlation_id"`
}

// StepHandler defines the interface for saga step execution
type StepHandler interface {
	// Execute performs the step action
	Execute(ctx context.Context, saga *Saga, step *Step) error

	// Compensate reverses the step action
	Compensate(ctx context.Context, saga *Saga, step *Step) error

	// GetName returns the step name
	GetName() string
}

// Store defines the interface for saga persistence
type Store interface {
	// Save persists a saga
	Save(ctx context.Context, saga *Saga) error

	// Get retrieves a saga by ID
	Get(ctx context.Context, id uuid.UUID) (*Saga, error)

	// UpdateStep updates a specific step in the saga
	UpdateStep(ctx context.Context, sagaID uuid.UUID, stepID string, updates map[string]interface{}) error

	// List retrieves sagas by status
	List(ctx context.Context, status Status, limit int) ([]*Saga, error)
}

// Coordinator orchestrates saga execution
type Coordinator struct {
	store      Store
	eventBus   events.EventBus
	handlers   map[string]StepHandler
	maxRetries int
	retryDelay time.Duration
}

// NewCoordinator creates a new saga coordinator
func NewCoordinator(store Store, eventBus events.EventBus) *Coordinator {
	return &Coordinator{
		store:      store,
		eventBus:   eventBus,
		handlers:   make(map[string]StepHandler),
		maxRetries: 3,
		retryDelay: time.Second * 5,
	}
}

// RegisterHandler registers a step handler
func (c *Coordinator) RegisterHandler(handler StepHandler) {
	c.handlers[handler.GetName()] = handler
}

// CreateSaga creates a new saga
func (c *Coordinator) CreateSaga(ctx context.Context, name string, organizationID uuid.UUID, steps []string) (*Saga, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "saga.create")
	defer span.End()

	sagaID := uuid.New()
	correlationID := uuid.New().String()

	// Create steps
	sagaSteps := make([]Step, len(steps))
	for i, stepName := range steps {
		sagaSteps[i] = Step{
			ID:         fmt.Sprintf("%s-%d", sagaID.String(), i),
			Name:       stepName,
			Status:     StepStatusPending,
			Metadata:   make(map[string]interface{}),
			MaxRetries: c.maxRetries,
		}
	}

	// Create saga
	saga := &Saga{
		ID:               sagaID,
		Name:             name,
		Status:           StatusPending,
		Steps:            sagaSteps,
		CurrentStepIndex: 0,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Metadata:         make(map[string]interface{}),
		OrganizationID:   organizationID,
		CorrelationID:    correlationID,
	}

	// Save saga
	if err := c.store.Save(ctx, saga); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to save saga", err)
		return nil, errors.Wrap(err, "failed to save saga")
	}

	// Publish saga created event
	event := events.NewDomainEvent(events.EventType("saga.created"), sagaID, "Saga", organizationID).
		WithCorrelation(correlationID).
		WithMetadata("saga_name", name).
		WithMetadata("steps", steps)

	if err := c.eventBus.Publish(ctx, event); err != nil {
		logger.Errorf("Failed to publish saga created event: %v", err)
	}

	logger.Infof("Created saga %s with %d steps", sagaID, len(steps))
	return saga, nil
}

// Execute runs the saga
func (c *Coordinator) Execute(ctx context.Context, sagaID uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "saga.execute")
	defer span.End()

	// Get saga
	saga, err := c.store.Get(ctx, sagaID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get saga", err)
		return errors.Wrap(err, "failed to get saga")
	}

	// Update saga status
	saga.Status = StatusRunning
	saga.UpdatedAt = time.Now()
	if err := c.store.Save(ctx, saga); err != nil {
		return errors.Wrap(err, "failed to update saga status")
	}

	// Execute steps
	for i := saga.CurrentStepIndex; i < len(saga.Steps); i++ {
		step := &saga.Steps[i]

		if err := c.executeStep(ctx, saga, step); err != nil {
			logger.Errorf("Step %s failed: %v", step.Name, err)

			// Update saga status to failed
			saga.Status = StatusFailed
			saga.UpdatedAt = time.Now()
			c.store.Save(ctx, saga)

			// Start compensation
			return c.compensate(ctx, saga)
		}

		// Update current step index
		saga.CurrentStepIndex = i + 1
		saga.UpdatedAt = time.Now()
		if err := c.store.Save(ctx, saga); err != nil {
			return errors.Wrap(err, "failed to update saga progress")
		}
	}

	// Mark saga as completed
	now := time.Now()
	saga.Status = StatusCompleted
	saga.CompletedAt = &now
	saga.UpdatedAt = now
	if err := c.store.Save(ctx, saga); err != nil {
		return errors.Wrap(err, "failed to mark saga as completed")
	}

	// Publish saga completed event
	event := events.NewDomainEvent(events.EventType("saga.completed"), sagaID, "Saga", saga.OrganizationID).
		WithCorrelation(saga.CorrelationID)

	if err := c.eventBus.Publish(ctx, event); err != nil {
		logger.Errorf("Failed to publish saga completed event: %v", err)
	}

	logger.Infof("Saga %s completed successfully", sagaID)
	return nil
}

// executeStep executes a single saga step
func (c *Coordinator) executeStep(ctx context.Context, saga *Saga, step *Step) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, fmt.Sprintf("saga.execute_step.%s", step.Name))
	defer span.End()

	// Get handler
	handler, exists := c.handlers[step.Name]
	if !exists {
		err := fmt.Errorf("no handler registered for step %s", step.Name)
		libOpentelemetry.HandleSpanError(&span, "Handler not found", err)
		return err
	}

	// Update step status
	now := time.Now()
	step.Status = StepStatusRunning
	step.StartedAt = &now
	c.store.UpdateStep(ctx, saga.ID, step.ID, map[string]interface{}{
		"status":     step.Status,
		"started_at": step.StartedAt,
	})

	// Execute with retries
	var lastErr error
	for attempt := 0; attempt <= step.MaxRetries; attempt++ {
		if attempt > 0 {
			logger.Warnf("Retrying step %s (attempt %d/%d)", step.Name, attempt, step.MaxRetries)
			time.Sleep(c.retryDelay)
		}

		if err := handler.Execute(ctx, saga, step); err != nil {
			lastErr = err
			step.RetryCount = attempt
			continue
		}

		// Success
		now = time.Now()
		step.Status = StepStatusCompleted
		step.CompletedAt = &now
		step.Error = nil

		c.store.UpdateStep(ctx, saga.ID, step.ID, map[string]interface{}{
			"status":       step.Status,
			"completed_at": step.CompletedAt,
			"retry_count":  step.RetryCount,
			"error":        nil,
		})

		logger.Infof("Step %s completed successfully", step.Name)
		return nil
	}

	// Failed after all retries
	step.Status = StepStatusFailed
	errStr := lastErr.Error()
	step.Error = &errStr

	c.store.UpdateStep(ctx, saga.ID, step.ID, map[string]interface{}{
		"status":      step.Status,
		"error":       step.Error,
		"retry_count": step.RetryCount,
	})

	libOpentelemetry.HandleSpanError(&span, fmt.Sprintf("Step %s failed after %d retries", step.Name, step.MaxRetries), lastErr)
	return lastErr
}

// compensate reverses completed steps
func (c *Coordinator) compensate(ctx context.Context, saga *Saga) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "saga.compensate")
	defer span.End()

	logger.Warnf("Starting compensation for saga %s", saga.ID)

	// Update saga status
	saga.Status = StatusCompensating
	saga.UpdatedAt = time.Now()
	c.store.Save(ctx, saga)

	// Compensate in reverse order
	compensationFailed := false
	for i := saga.CurrentStepIndex - 1; i >= 0; i-- {
		step := &saga.Steps[i]

		// Skip steps that weren't completed
		if step.Status != StepStatusCompleted {
			continue
		}

		// Get handler
		handler, exists := c.handlers[step.Name]
		if !exists {
			logger.Errorf("No handler for step %s during compensation", step.Name)
			compensationFailed = true
			continue
		}

		// Compensate step
		if err := handler.Compensate(ctx, saga, step); err != nil {
			logger.Errorf("Failed to compensate step %s: %v", step.Name, err)
			compensationFailed = true
			continue
		}

		// Update step status
		now := time.Now()
		step.Status = StepStatusCompensated
		step.CompensatedAt = &now

		c.store.UpdateStep(ctx, saga.ID, step.ID, map[string]interface{}{
			"status":         step.Status,
			"compensated_at": step.CompensatedAt,
		})

		logger.Infof("Compensated step %s", step.Name)
	}

	// Update saga status
	if compensationFailed {
		saga.Status = StatusAborted
	} else {
		saga.Status = StatusCompensated
	}
	saga.UpdatedAt = time.Now()
	c.store.Save(ctx, saga)

	// Publish compensation event
	eventType := events.EventType("saga.compensated")
	if compensationFailed {
		eventType = events.EventType("saga.aborted")
	}

	event := events.NewDomainEvent(eventType, saga.ID, "Saga", saga.OrganizationID).
		WithCorrelation(saga.CorrelationID)

	if err := c.eventBus.Publish(ctx, event); err != nil {
		logger.Errorf("Failed to publish compensation event: %v", err)
	}

	if compensationFailed {
		return errors.New("compensation failed for some steps")
	}

	return nil
}

// Resume resumes a failed or pending saga
func (c *Coordinator) Resume(ctx context.Context, sagaID uuid.UUID) error {
	saga, err := c.store.Get(ctx, sagaID)
	if err != nil {
		return errors.Wrap(err, "failed to get saga")
	}

	// Only resume if saga is in appropriate state
	switch saga.Status {
	case StatusFailed, StatusPending:
		return c.Execute(ctx, sagaID)
	case StatusCompensating:
		return c.compensate(ctx, saga)
	default:
		return fmt.Errorf("cannot resume saga in status %s", saga.Status)
	}
}
