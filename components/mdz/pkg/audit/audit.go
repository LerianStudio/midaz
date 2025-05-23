package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a single audit log entry
type Entry struct {
	ID          string            `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Flags       map[string]string `json:"flags"`
	User        string            `json:"user"`
	Result      string            `json:"result"`
	Error       string            `json:"error,omitempty"`
	Duration    time.Duration     `json:"duration"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Undoable    bool              `json:"undoable"`
	UndoCommand string            `json:"undo_command,omitempty"`
}

// Trail represents the audit trail manager
type Trail struct {
	mu          sync.RWMutex
	file        string
	maxEntries  int
	entries     []Entry
	initialized bool
}

// Config represents audit trail configuration
type Config struct {
	Enabled    bool
	FilePath   string
	MaxEntries int
}

// DefaultConfig returns default audit trail configuration
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()

	return &Config{
		Enabled:    true,
		FilePath:   filepath.Join(homeDir, ".mdz", "audit.json"),
		MaxEntries: 10000,
	}
}

// New creates a new audit trail
func New(config *Config) (*Trail, error) {
	if config == nil {
		config = DefaultConfig()
	}

	trail := &Trail{
		file:       config.FilePath,
		maxEntries: config.MaxEntries,
		entries:    make([]Entry, 0),
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(config.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create audit directory: %w", err)
	}

	// Load existing entries
	if err := trail.load(); err != nil {
		// If file doesn't exist, that's okay
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load audit trail: %w", err)
		}
	}

	trail.initialized = true

	return trail, nil
}

// LogCommand logs a command execution
func (t *Trail) LogCommand(entry Entry) error {
	if !t.initialized {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Generate ID if not provided
	if entry.ID == "" {
		entry.ID = generateID()
	}

	// Set timestamp if not provided
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Set user if not provided
	if entry.User == "" {
		entry.User = os.Getenv("USER")
	}

	// Add to entries
	t.entries = append(t.entries, entry)

	// Trim if exceeds max entries
	if len(t.entries) > t.maxEntries {
		t.entries = t.entries[len(t.entries)-t.maxEntries:]
	}

	// Save to file
	return t.save()
}

// GetHistory returns command history
func (t *Trail) GetHistory(limit int) []Entry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if limit <= 0 || limit > len(t.entries) {
		limit = len(t.entries)
	}

	// Return most recent entries
	start := len(t.entries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]Entry, limit)
	copy(result, t.entries[start:])

	// Reverse to show most recent first
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}

	return result
}

// GetEntry returns a specific entry by ID
func (t *Trail) GetEntry(id string) (*Entry, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, entry := range t.entries {
		if entry.ID == id {
			return &entry, nil
		}
	}

	return nil, fmt.Errorf("entry not found: %s", id)
}

// GetUndoableCommands returns commands that can be undone
func (t *Trail) GetUndoableCommands(limit int) []Entry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var undoable []Entry
	for i := len(t.entries) - 1; i >= 0 && len(undoable) < limit; i-- {
		if t.entries[i].Undoable && t.entries[i].Result == "success" {
			undoable = append(undoable, t.entries[i])
		}
	}

	return undoable
}

// Clear clears the audit trail
func (t *Trail) Clear() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.entries = make([]Entry, 0)

	return t.save()
}

// load loads entries from file
func (t *Trail) load() error {
	data, err := os.ReadFile(t.file)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &t.entries)
}

// save saves entries to file
func (t *Trail) save() error {
	data, err := json.MarshalIndent(t.entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(t.file, data, 0644)
}

// generateID generates a unique ID for an entry
func generateID() string {
	return fmt.Sprintf("%d-%d", time.Now().Unix(), time.Now().Nanosecond())
}

// Builder provides a fluent interface for building audit entries
type Builder struct {
	entry Entry
}

// NewBuilder creates a new audit entry builder
func NewBuilder() *Builder {
	return &Builder{
		entry: Entry{
			Flags:    make(map[string]string),
			Metadata: make(map[string]string),
		},
	}
}

// WithCommand sets the command
func (b *Builder) WithCommand(cmd string) *Builder {
	b.entry.Command = cmd
	return b
}

// WithArgs sets the arguments
func (b *Builder) WithArgs(args []string) *Builder {
	b.entry.Args = args
	return b
}

// WithFlag adds a flag
func (b *Builder) WithFlag(key, value string) *Builder {
	b.entry.Flags[key] = value
	return b
}

// WithFlags adds multiple flags
func (b *Builder) WithFlags(flags map[string]string) *Builder {
	for k, v := range flags {
		b.entry.Flags[k] = v
	}

	return b
}

// WithResult sets the result
func (b *Builder) WithResult(result string) *Builder {
	b.entry.Result = result
	return b
}

// WithError sets the error
func (b *Builder) WithError(err error) *Builder {
	if err != nil {
		b.entry.Error = err.Error()
		b.entry.Result = "error"
	}

	return b
}

// WithDuration sets the duration
func (b *Builder) WithDuration(duration time.Duration) *Builder {
	b.entry.Duration = duration
	return b
}

// WithMetadata adds metadata
func (b *Builder) WithMetadata(key, value string) *Builder {
	b.entry.Metadata[key] = value
	return b
}

// WithUndo marks the command as undoable with the undo command
func (b *Builder) WithUndo(undoCommand string) *Builder {
	b.entry.Undoable = true
	b.entry.UndoCommand = undoCommand

	return b
}

// Build returns the built entry
func (b *Builder) Build() Entry {
	return b.entry
}
