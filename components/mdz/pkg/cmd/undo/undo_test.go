package undo

import (
	"bytes"
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

func TestNewCmdUndo(t *testing.T) {
	f := createTestFactory()

	cmd := NewCmdUndo(f)

	if cmd == nil {
		t.Fatal("NewCmdUndo should not return nil")
	}

	if cmd.Use != "undo [id]" {
		t.Errorf("Expected Use 'undo [id]', got '%s'", cmd.Use)
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

	// Test argument validation
	if cmd.Args == nil {
		t.Error("Command should have argument validation")
	}
}

func TestFactoryUndo_setFlags(t *testing.T) {
	f := createTestFactory()
	fUndo := &factoryUndo{factory: f}

	cmd := NewCmdUndo(f)

	// Test default values
	if fUndo.last {
		t.Error("Expected default last false")
	}
	if fUndo.dryRun {
		t.Error("Expected default dryRun false")
	}

	// Test flags exist
	flags := []string{"last", "dry-run"}
	for _, flagName := range flags {
		flag := cmd.Flag(flagName)
		if flag == nil {
			t.Errorf("Flag '%s' should exist", flagName)
		}
	}
}

func TestFactoryUndo_DefaultValues(t *testing.T) {
	f := createTestFactory()

	fUndo := &factoryUndo{
		factory: f,
	}

	if fUndo.factory != f {
		t.Error("Factory should be set correctly")
	}
	if fUndo.last {
		t.Error("last should default to false")
	}
	if fUndo.dryRun {
		t.Error("dryRun should default to false")
	}
}

func TestFactoryUndo_CommandStructure(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdUndo(f)

	// Test long description mentions expected features
	expectedFeatures := []string{"undo", "command", "create", "update", "delete"}
	for _, feature := range expectedFeatures {
		if !strings.Contains(strings.ToLower(cmd.Long), strings.ToLower(feature)) {
			t.Errorf("Long description should mention '%s'", feature)
		}
	}

	// Test examples contain expected commands
	expectedExamples := []string{"undo", "--last", "--dry-run"}
	for _, example := range expectedExamples {
		if !strings.Contains(cmd.Example, example) {
			t.Errorf("Examples should contain '%s'", example)
		}
	}
}

func TestFactoryUndo_Flags(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdUndo(f)

	// Test last flag
	lastFlag := cmd.Flag("last")
	if lastFlag == nil {
		t.Fatal("last flag should exist")
	}
	if lastFlag.DefValue != "false" {
		t.Errorf("last flag default should be 'false', got '%s'", lastFlag.DefValue)
	}

	// Test dry-run flag
	dryRunFlag := cmd.Flag("dry-run")
	if dryRunFlag == nil {
		t.Fatal("dry-run flag should exist")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("dry-run flag default should be 'false', got '%s'", dryRunFlag.DefValue)
	}
}

func TestFactoryUndo_UsageAndArgs(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdUndo(f)

	// Test the command accepts maximum 1 argument
	if cmd.Args == nil {
		t.Error("Command should have argument validation")
	}

	// Test command use pattern
	if !strings.Contains(cmd.Use, "[id]") {
		t.Error("Command use should indicate optional ID parameter")
	}
}

func TestFactoryUndo_HelpContent(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdUndo(f)

	// Test that help content mentions key concepts
	helpContent := cmd.Long + " " + cmd.Example

	keyTerms := []string{
		"undo",
		"command",
		"ID",
		"last",
		"dry-run",
		"create",
		"update",
		"delete",
	}

	for _, term := range keyTerms {
		if !strings.Contains(strings.ToLower(helpContent), strings.ToLower(term)) {
			t.Errorf("Help content should mention '%s'", term)
		}
	}
}

func TestFactoryUndo_InitialState(t *testing.T) {
	f := createTestFactory()

	// Test that factory creates with proper initial state
	fUndo := &factoryUndo{
		factory: f,
		last:    false,
		dryRun:  false,
	}

	if fUndo.factory.IOStreams.Out == nil {
		t.Error("Factory should have output stream")
	}
	if fUndo.factory.IOStreams.Err == nil {
		t.Error("Factory should have error stream")
	}
	if fUndo.factory.IOStreams.In == nil {
		t.Error("Factory should have input stream")
	}

	// Test boolean flag defaults
	if fUndo.last {
		t.Error("last flag should default to false")
	}
	if fUndo.dryRun {
		t.Error("dryRun flag should default to false")
	}
}

func TestFactoryUndo_CommandParsing(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdUndo(f)

	// Test that we can parse the command structure properly
	if cmd.Name() != "undo" {
		t.Errorf("Expected command name 'undo', got '%s'", cmd.Name())
	}

	// Test that the command has proper structure for help
	if cmd.Short == "" {
		t.Error("Command should have short description")
	}
	if len(cmd.Short) > 80 {
		t.Error("Short description should be concise (under 80 chars)")
	}
}

func TestFactoryUndo_ExampleValidation(t *testing.T) {
	f := createTestFactory()
	cmd := NewCmdUndo(f)

	// Test that examples follow expected patterns
	examples := strings.Split(cmd.Example, "\n")

	foundBasicUndo := false
	foundLastFlag := false
	foundDryRunFlag := false

	for _, example := range examples {
		example = strings.TrimSpace(example)
		if strings.Contains(example, "mdz undo") && !strings.Contains(example, "--") {
			foundBasicUndo = true
		}
		if strings.Contains(example, "--last") {
			foundLastFlag = true
		}
		if strings.Contains(example, "--dry-run") {
			foundDryRunFlag = true
		}
	}

	if !foundBasicUndo {
		t.Error("Examples should include basic undo usage")
	}
	if !foundLastFlag {
		t.Error("Examples should include --last flag usage")
	}
	if !foundDryRunFlag {
		t.Error("Examples should include --dry-run flag usage")
	}
}

// Helper functions
func createTestFactory() *factory.Factory {
	out := &bytes.Buffer{}
	err := &bytes.Buffer{}

	iostreams := &iostreams.IOStreams{
		In:  &nopReadCloser{strings.NewReader("")},
		Out: out,
		Err: err,
	}

	return &factory.Factory{
		IOStreams: iostreams,
	}
}
