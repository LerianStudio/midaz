package iostreams

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSystem(t *testing.T) {
	// Get the system IO streams
	ios := System()

	// Verify the IO streams are set correctly
	assert.Equal(t, os.Stdin, ios.In)
	assert.Equal(t, os.Stdout, ios.Out)
	assert.Equal(t, os.Stderr, ios.Err)
}

func TestIOStreamsCustomization(t *testing.T) {
	// Create custom IO streams
	customIn := io.NopCloser(bytes.NewBufferString("test input"))
	customOut := &bytes.Buffer{}
	customErr := &bytes.Buffer{}

	// Create custom IO streams
	ios := &IOStreams{
		In:  customIn,
		Out: customOut,
		Err: customErr,
	}

	// Verify the IO streams are set correctly
	assert.Equal(t, customIn, ios.In)
	assert.Equal(t, customOut, ios.Out)
	assert.Equal(t, customErr, ios.Err)

	// Test writing to Out and Err
	_, err := io.WriteString(ios.Out, "test output")
	assert.NoError(t, err)
	assert.Equal(t, "test output", customOut.String())

	_, err = io.WriteString(ios.Err, "test error")
	assert.NoError(t, err)
	assert.Equal(t, "test error", customErr.String())

	// Test reading from In
	buf := make([]byte, 10)
	n, err := ios.In.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Equal(t, "test input", string(buf))
}

func TestIOStreamsWithNilValues(t *testing.T) {
	// Create IO streams with nil values
	ios := &IOStreams{
		In:  nil,
		Out: nil,
		Err: nil,
	}

	// Verify the IO streams are nil
	assert.Nil(t, ios.In)
	assert.Nil(t, ios.Out)
	assert.Nil(t, ios.Err)
}
