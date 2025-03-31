package output

import (
	"bytes"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestFormatAndPrint(t *testing.T) {
	// Create a buffer to capture output
	buf := &bytes.Buffer{}

	// Test cases
	testCases := []struct {
		name       string
		noColor    bool
		id         string
		entity     string
		method     string
		wantOutput string
	}{
		{
			name:       "Created with color",
			noColor:    false,
			id:         "123",
			entity:     "Account",
			method:     Created,
			wantOutput: "The Account 123 has been successfully created.\n",
		},
		{
			name:       "Deleted with color",
			noColor:    false,
			id:         "456",
			entity:     "Transaction",
			method:     Deleted,
			wantOutput: "The Transaction 456 has been successfully deleted.\n",
		},
		{
			name:       "Updated with color",
			noColor:    false,
			id:         "789",
			entity:     "Ledger",
			method:     Updated,
			wantOutput: "The Ledger 789 has been successfully updated.\n",
		},
		{
			name:       "Custom method with color",
			noColor:    false,
			id:         "abc",
			entity:     "Operation",
			method:     "processed",
			wantOutput: "The Operation abc has been successfully processed.\n",
		},
		{
			name:       "Created without color",
			noColor:    true,
			id:         "123",
			entity:     "Account",
			method:     Created,
			wantOutput: "The Account 123 has been successfully created.\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset buffer
			buf.Reset()

			// Create factory with custom iostreams
			ios := &iostreams.IOStreams{
				Out: buf,
			}
			f := &factory.Factory{
				IOStreams: ios,
				Flags: factory.Flags{
					NoColor: tc.noColor,
				},
			}

			// Call FormatAndPrint
			FormatAndPrint(f, tc.id, tc.entity, tc.method)

			// Check output
			assert.Equal(t, tc.wantOutput, buf.String())
		})
	}
}

func TestPrintf(t *testing.T) {
	// Create a buffer to capture output
	buf := &bytes.Buffer{}

	// Test cases
	testCases := []struct {
		name       string
		format     string
		args       []interface{}
		wantOutput string
	}{
		{
			name:       "Simple message",
			format:     "Hello, %s!",
			args:       []interface{}{"world"},
			wantOutput: "Hello, world!\n",
		},
		{
			name:       "Multiple arguments",
			format:     "%d + %d = %d",
			args:       []interface{}{1, 2, 3},
			wantOutput: "1 + 2 = 3\n",
		},
		{
			name:       "No arguments",
			format:     "Just a message",
			args:       []interface{}{},
			wantOutput: "Just a message\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset buffer
			buf.Reset()

			// Call Printf
			Printf(buf, tc.format, tc.args...)

			// Check output
			assert.Equal(t, tc.wantOutput, buf.String())
		})
	}
}

func TestErrorf(t *testing.T) {
	// Create a buffer to capture output
	buf := &bytes.Buffer{}

	// Test cases
	testCases := []struct {
		name       string
		err        error
		wantOutput string
	}{
		{
			name:       "Simple error",
			err:        errors.New("something went wrong"),
			wantOutput: "something went wrong\n",
		},
		{
			name:       "Nil error",
			err:        nil,
			wantOutput: "%!s(<nil>)\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset buffer
			buf.Reset()

			// Call Errorf
			returnedErr := Errorf(buf, tc.err)

			// Check output
			assert.Equal(t, tc.wantOutput, buf.String())

			// Check returned error
			assert.Equal(t, tc.err, returnedErr)
		})
	}
}

func TestNewTable(t *testing.T) {
	// Create a buffer to capture output
	buf := &bytes.Buffer{}

	// Create a table
	table := NewTable(buf)

	// Check table properties
	assert.NotNil(t, table)

	// Test the table by rendering some data
	table.SetHeader([]string{"ID", "Name"})
	table.Append([]string{"1", "Alice"})
	table.Append([]string{"2", "Bob"})
	table.Render()

	// Check that output contains the expected data
	output := buf.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "1")
	assert.Contains(t, output, "Alice")
	assert.Contains(t, output, "2")
	assert.Contains(t, output, "Bob")
}

func TestGeneralOutput(t *testing.T) {
	// Create a buffer to capture output
	buf := &bytes.Buffer{}

	// Create a general output
	g := GeneralOutput{
		Msg: "Test message",
		Out: buf,
	}

	// Call Output
	g.Output()

	// Check output
	assert.Equal(t, "Test message\n", buf.String())
}

func TestErrorOutput(t *testing.T) {
	// Create a buffer to capture output
	buf := &bytes.Buffer{}

	// Create an error output
	err := errors.New("test error")
	e := ErrorOutput{
		GeneralOutput: GeneralOutput{
			Out: buf,
		},
		Err: err,
	}

	// Call Output
	returnedErr := e.Output()

	// Check output
	assert.Equal(t, "test error\n", buf.String())

	// Check returned error
	assert.Equal(t, err, returnedErr)

	// Test with nil error
	buf.Reset()
	e.Err = nil
	returnedErr = e.Output()

	// Check output
	assert.Equal(t, "%!s(<nil>)\n", buf.String())

	// Check returned error
	assert.Nil(t, returnedErr)
}
