// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Command yamljson converts a YAML document to JSON while preserving mapping
// order. The documentation pipeline uses it instead of carrying a Node YAML
// parser solely for format conversion.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 && len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: yamljson <input.yaml> [output.json]")
		os.Exit(2)
	}

	input, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open input: %v\n", err)
		os.Exit(1)
	}
	defer input.Close()

	var output bytes.Buffer
	if err = convert(input, &output); err != nil {
		fmt.Fprintf(os.Stderr, "convert YAML to JSON: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) == 3 {
		if err = os.WriteFile(os.Args[2], output.Bytes(), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write output: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if _, err = output.WriteTo(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "write output: %v\n", err)
		os.Exit(1)
	}
}

func convert(input io.Reader, output io.Writer) error {
	decoder := yaml.NewDecoder(input)
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if len(document.Content) != 1 {
		return errors.New("expected exactly one YAML document")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("expected exactly one YAML document")
	}

	var jsonDocument bytes.Buffer
	if err := writeJSON(&jsonDocument, document.Content[0], 0); err != nil {
		return err
	}
	jsonDocument.WriteByte('\n')

	_, err := io.Copy(output, &jsonDocument)
	return err
}

func writeJSON(output *bytes.Buffer, node *yaml.Node, depth int) error {
	switch node.Kind {
	case yaml.MappingNode:
		output.WriteByte('{')
		for i := 0; i < len(node.Content); i += 2 {
			if i > 0 {
				output.WriteByte(',')
			}
			writeIndent(output, depth+1)

			key := node.Content[i]
			if key.Kind != yaml.ScalarNode {
				return errors.New("mapping key is not a scalar")
			}
			encodedKey, err := marshalJSON(key.Value)
			if err != nil {
				return err
			}
			output.Write(encodedKey)
			output.WriteString(": ")

			if err = writeJSON(output, node.Content[i+1], depth+1); err != nil {
				return err
			}
		}
		if len(node.Content) > 0 {
			writeIndent(output, depth)
		}
		output.WriteByte('}')
	case yaml.SequenceNode:
		output.WriteByte('[')
		for i, child := range node.Content {
			if i > 0 {
				output.WriteByte(',')
			}
			writeIndent(output, depth+1)
			if err := writeJSON(output, child, depth+1); err != nil {
				return err
			}
		}
		if len(node.Content) > 0 {
			writeIndent(output, depth)
		}
		output.WriteByte(']')
	case yaml.ScalarNode:
		var value any
		if err := node.Decode(&value); err != nil {
			return err
		}
		encodedValue, err := marshalJSON(value)
		if err != nil {
			return err
		}
		output.Write(encodedValue)
	case yaml.AliasNode:
		return writeJSON(output, node.Alias, depth)
	default:
		return fmt.Errorf("unsupported YAML node kind %d", node.Kind)
	}

	return nil
}

func marshalJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(output.Bytes(), []byte{'\n'}), nil
}

func writeIndent(output *bytes.Buffer, depth int) {
	output.WriteByte('\n')
	output.Write(bytes.Repeat([]byte("  "), depth))
}
