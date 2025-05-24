package login

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
)

// nopReadCloser wraps a Reader to implement ReadCloser
type nopReadCloser struct {
	io.Reader
}

func (nopReadCloser) Close() error { return nil }

func TestValidateCredentials_EmptyUsername(t *testing.T) {
	err := validateCredentials("", "password")
	if err == nil {
		t.Error("Expected error for empty username")
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("Expected error to mention 'empty', got '%s'", err.Error())
	}
}

func TestValidateCredentials_EmptyPassword(t *testing.T) {
	err := validateCredentials("user", "")
	if err == nil {
		t.Error("Expected error for empty password")
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("Expected error to mention 'empty', got '%s'", err.Error())
	}
}

func TestValidateCredentials_Valid(t *testing.T) {
	err := validateCredentials("user", "password")
	if err != nil {
		t.Errorf("Expected no error for valid credentials, got %v", err)
	}
}

func TestValidateCredentials_BothEmpty(t *testing.T) {
	err := validateCredentials("", "")
	if err == nil {
		t.Error("Expected error when both username and password are empty")
	}
}

func TestLoginCommand_FlagDefaults(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdLogin(f)

	// Test username flag
	usernameFlag := cmd.Flag("username")
	if usernameFlag == nil {
		t.Fatal("username flag should exist")
	}

	if usernameFlag.DefValue != "" {
		t.Errorf("username flag default should be empty, got '%s'", usernameFlag.DefValue)
	}

	// Test password flag
	passwordFlag := cmd.Flag("password")
	if passwordFlag == nil {
		t.Fatal("password flag should exist")
	}

	if passwordFlag.DefValue != "" {
		t.Errorf("password flag default should be empty, got '%s'", passwordFlag.DefValue)
	}

	// Test browser flag
	browserFlag := cmd.Flag("browser")
	if browserFlag == nil {
		t.Fatal("browser flag should exist")
	}

	if browserFlag.DefValue != "false" {
		t.Errorf("browser flag default should be 'false', got '%s'", browserFlag.DefValue)
	}
}

func TestLoginCommand_Help(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdLogin(f)

	// Test that help content mentions key concepts
	helpContent := cmd.Long + " " + cmd.Example

	keyTerms := []string{
		"login",
		"authentication",
		"credentials",
		"username",
		"password",
		"browser",
	}

	for _, term := range keyTerms {
		if !strings.Contains(strings.ToLower(helpContent), strings.ToLower(term)) {
			t.Errorf("Help content should mention '%s'", term)
		}
	}
}

func TestLoginCommand_Examples(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdLogin(f)

	// Test that examples include expected usage patterns
	examples := strings.Split(cmd.Example, "\n")

	foundBasicLogin := false
	foundUsernameFlag := false
	foundBrowserFlag := false

	for _, example := range examples {
		example = strings.TrimSpace(example)
		if strings.Contains(example, "mdz login") && !strings.Contains(example, "--") {
			foundBasicLogin = true
		}

		if strings.Contains(example, "--username") {
			foundUsernameFlag = true
		}

		if strings.Contains(example, "--browser") {
			foundBrowserFlag = true
		}
	}

	if !foundBasicLogin {
		t.Error("Examples should include basic login usage")
	}

	if !foundUsernameFlag {
		t.Error("Examples should include --username flag usage")
	}

	if !foundBrowserFlag {
		t.Error("Examples should include --browser flag usage")
	}
}

func TestInitializeContext(t *testing.T) {
	// Test that initializeContext doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("initializeContext should not panic: %v", r)
		}
	}()

	initializeContext()

	// Test that context variables are set
	if srvCallBackCtx == nil {
		t.Error("srvCallBackCtx should not be nil after initialization")
	}

	if srvCallBackCancel == nil {
		t.Error("srvCallBackCancel should not be nil after initialization")
	}

	// Test that cancel function works
	srvCallBackCancel()

	// After cancellation, Done() should be closed
	select {
	case <-srvCallBackCtx.Done():
		// Expected behavior
	default:
		t.Error("Context should be cancelled after calling cancel function")
	}
}

func TestBrowserStruct(t *testing.T) {
	browserVal := browser{
		Err: nil,
	}

	if browserVal.Err != nil {
		t.Error("Browser error should be nil when not set")
	}

	// Test with error
	browserVal.Err = errors.New("test error")

	if browserVal.Err == nil {
		t.Error("Browser error should be set correctly")
	}
}

func TestLoginCommand_Structure(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdLogin(f)

	// Test command structure
	if cmd.Use != "login" {
		t.Errorf("Expected Use 'login', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Command should have a short description")
	}

	if cmd.Long == "" {
		t.Error("Command should have a long description")
	}

	if cmd.Example == "" {
		t.Error("Command should have examples")
	}

	if cmd.RunE == nil {
		t.Error("Command should have a RunE function")
	}
}

// Helper functions
func createTestFactory() *factory.Factory {
	out := &bytes.Buffer{}
	err := &bytes.Buffer{}

	ioStreams := &iostreams.IOStreams{
		In:  &nopReadCloser{strings.NewReader("")},
		Out: out,
		Err: err,
	}

	return &factory.Factory{
		IOStreams: ioStreams,
	}
}
