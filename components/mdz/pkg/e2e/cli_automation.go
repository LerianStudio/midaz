package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// CLISession represents an interactive CLI session similar to Playwright's Page
type CLISession struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	context    context.Context
	cancel     context.CancelFunc
	recorder   *SessionRecorder
	config     *SessionConfig
	outputBuf  *SafeBuffer
	errorBuf   *SafeBuffer
	wg         sync.WaitGroup
	closed     bool
	mu         sync.Mutex
}

// SessionConfig configures the CLI session behavior
type SessionConfig struct {
	Command     string
	Args        []string
	Env         map[string]string
	WorkingDir  string
	Timeout     time.Duration
	RecordPath  string
	Interactive bool
	Debug       bool
}

// SessionRecorder captures all interactions for analysis and replay
type SessionRecorder struct {
	Events    []SessionEvent `json:"events"`
	Metadata  SessionMeta    `json:"metadata"`
	mu        sync.Mutex
	startTime time.Time
}

// SessionEvent represents a single interaction event
type SessionEvent struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
	Context   string    `json:"context,omitempty"`
	Delay     int64     `json:"delay_ms"`
}

// SessionMeta contains session metadata
type SessionMeta struct {
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Duration    int64     `json:"duration_ms"`
	Command     string    `json:"command"`
	Args        []string  `json:"args"`
	Environment []string  `json:"environment"`
	ExitCode    int       `json:"exit_code"`
	Success     bool      `json:"success"`
}

// SafeBuffer provides thread-safe buffer operations
type SafeBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (sb *SafeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *SafeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func (sb *SafeBuffer) Bytes() []byte {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Bytes()
}

func (sb *SafeBuffer) Reset() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.buf.Reset()
}

// NewCLISession creates a new CLI automation session
func NewCLISession(config *SessionConfig) (*CLISession, error) {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)

	recorder := &SessionRecorder{
		Events:    make([]SessionEvent, 0),
		startTime: time.Now(),
		Metadata: SessionMeta{
			StartTime:   time.Now(),
			Command:     config.Command,
			Args:        config.Args,
			Environment: os.Environ(),
		},
	}

	session := &CLISession{
		context:   ctx,
		cancel:    cancel,
		recorder:  recorder,
		config:    config,
		outputBuf: &SafeBuffer{},
		errorBuf:  &SafeBuffer{},
	}

	return session, nil
}

// Start begins the CLI session
func (s *CLISession) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil {
		return fmt.Errorf("session already started")
	}

	s.cmd = exec.CommandContext(s.context, s.config.Command, s.config.Args...)
	
	if s.config.WorkingDir != "" {
		s.cmd.Dir = s.config.WorkingDir
	}

	// Set environment variables
	s.cmd.Env = os.Environ()
	for k, v := range s.config.Env {
		s.cmd.Env = append(s.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create pipes for interactive communication
	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	s.stdin = stdin

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	s.stdout = stdout

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	s.stderr = stderr

	// Start the command
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	s.recorder.recordEvent("session_start", fmt.Sprintf("Started %s %v", s.config.Command, s.config.Args), "")

	// Start output readers
	s.wg.Add(2)
	go s.readOutput()
	go s.readError()

	return nil
}

// Type sends text input to the CLI session
func (s *CLISession) Type(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stdin == nil {
		return fmt.Errorf("session not started or stdin not available")
	}

	_, err := s.stdin.Write([]byte(text))
	if err != nil {
		return fmt.Errorf("failed to write to stdin: %w", err)
	}

	s.recorder.recordEvent("input", text, "user_typing")
	return nil
}

// Press sends special keys (Enter, Tab, Ctrl+C, etc.)
func (s *CLISession) Press(key string) error {
	var keyBytes []byte
	
	switch strings.ToLower(key) {
	case "enter", "return":
		keyBytes = []byte("\n")
	case "tab":
		keyBytes = []byte("\t")
	case "ctrl+c", "sigint":
		keyBytes = []byte("\x03")
	case "ctrl+d", "eof":
		keyBytes = []byte("\x04")
	case "escape", "esc":
		keyBytes = []byte("\x1b")
	case "backspace":
		keyBytes = []byte("\x08")
	case "delete":
		keyBytes = []byte("\x7f")
	default:
		return fmt.Errorf("unsupported key: %s", key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stdin == nil {
		return fmt.Errorf("session not started or stdin not available")
	}

	_, err := s.stdin.Write(keyBytes)
	if err != nil {
		return fmt.Errorf("failed to write key to stdin: %w", err)
	}

	s.recorder.recordEvent("key_press", key, "special_key")
	return nil
}

// WaitForOutput waits for specific text to appear in stdout
func (s *CLISession) WaitForOutput(text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		output := s.outputBuf.String()
		if strings.Contains(output, text) {
			s.recorder.recordEvent("wait_success", fmt.Sprintf("Found: %s", text), "output_match")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	s.recorder.recordEvent("wait_timeout", fmt.Sprintf("Timeout waiting for: %s", text), "error")
	return fmt.Errorf("timeout waiting for output: %s", text)
}

// WaitForPrompt waits for a prompt pattern (e.g., "> ", "$ ", "Select: ")
func (s *CLISession) WaitForPrompt(prompt string, timeout time.Duration) error {
	return s.WaitForOutput(prompt, timeout)
}

// GetOutput returns the current stdout content
func (s *CLISession) GetOutput() string {
	return s.outputBuf.String()
}

// GetError returns the current stderr content
func (s *CLISession) GetError() string {
	return s.errorBuf.String()
}

// Screenshot captures current terminal state (text-based)
func (s *CLISession) Screenshot() *TerminalSnapshot {
	output := s.outputBuf.String()
	lines := strings.Split(output, "\n")
	
	// Get last 25 lines for terminal-like view
	start := 0
	if len(lines) > 25 {
		start = len(lines) - 25
	}
	
	return &TerminalSnapshot{
		Timestamp: time.Now(),
		Lines:     lines[start:],
		FullOutput: output,
		Cursor:    len(output),
	}
}

// TerminalSnapshot represents the current state of the terminal
type TerminalSnapshot struct {
	Timestamp  time.Time `json:"timestamp"`
	Lines      []string  `json:"lines"`
	FullOutput string    `json:"full_output"`
	Cursor     int       `json:"cursor_position"`
}

// Wait waits for a specified duration
func (s *CLISession) Wait(duration time.Duration) {
	time.Sleep(duration)
	s.recorder.recordEvent("wait", fmt.Sprintf("Waited %v", duration), "delay")
}

// Close terminates the CLI session
func (s *CLISession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Close stdin to signal end of input
	if s.stdin != nil {
		s.stdin.Close()
	}

	// Cancel context to terminate command if still running
	s.cancel()

	// Wait for output readers to finish
	s.wg.Wait()

	// Record session end
	s.recorder.Metadata.EndTime = time.Now()
	s.recorder.Metadata.Duration = s.recorder.Metadata.EndTime.Sub(s.recorder.Metadata.StartTime).Milliseconds()

	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Wait() // Wait for process to complete
		if s.cmd.ProcessState != nil {
			s.recorder.Metadata.ExitCode = s.cmd.ProcessState.ExitCode()
			s.recorder.Metadata.Success = s.cmd.ProcessState.Success()
		}
	}

	s.recorder.recordEvent("session_end", "Session closed", "")

	// Save recording if path specified
	if s.config.RecordPath != "" {
		return s.SaveRecording(s.config.RecordPath)
	}

	return nil
}

// SaveRecording saves the session recording to a file
func (s *CLISession) SaveRecording(path string) error {
	data, err := json.MarshalIndent(s.recorder, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal recording: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// readOutput continuously reads from stdout
func (s *CLISession) readOutput() {
	defer s.wg.Done()
	
	scanner := bufio.NewScanner(s.stdout)
	for scanner.Scan() {
		line := scanner.Text()
		s.outputBuf.Write([]byte(line + "\n"))
		
		if s.config.Debug {
			fmt.Printf("[OUT] %s\n", line)
		}
		
		s.recorder.recordEvent("output", line, "stdout")
	}
}

// readError continuously reads from stderr
func (s *CLISession) readError() {
	defer s.wg.Done()
	
	scanner := bufio.NewScanner(s.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		s.errorBuf.Write([]byte(line + "\n"))
		
		if s.config.Debug {
			fmt.Printf("[ERR] %s\n", line)
		}
		
		s.recorder.recordEvent("error", line, "stderr")
	}
}

// recordEvent adds an event to the recording
func (r *SessionRecorder) recordEvent(eventType, data, context string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	now := time.Now()
	delay := int64(0)
	if len(r.Events) > 0 {
		delay = now.Sub(r.Events[len(r.Events)-1].Timestamp).Milliseconds()
	}
	
	event := SessionEvent{
		Type:      eventType,
		Timestamp: now,
		Data:      data,
		Context:   context,
		Delay:     delay,
	}
	
	r.Events = append(r.Events, event)
}