// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutil

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// PostgreSQL wire protocol request codes carried in place of a protocol
// version by the untyped first message of a connection.
const (
	pgSSLRequestCode    = 80877103
	pgGSSENCRequestCode = 80877104
	pgCancelRequestCode = 80877102
)

// FakePostgres speaks just enough of the PostgreSQL v3 wire protocol for the
// tenant pool manager to open and ping a per-tenant pool: it trusts every
// startup, answers every simple query with an empty result and rejects the
// extended protocol. It lets the production pool manager resolve a tenant pool
// without a database; nothing reads from the pool. It speaks plaintext only,
// so the pool dial needs ALLOW_INSECURE_TLS=true.
type FakePostgres struct {
	listener net.Listener
	wg       sync.WaitGroup
}

// StartFakePostgres listens on a loopback port until the test ends.
func StartFakePostgres(t *testing.T) *FakePostgres {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &FakePostgres{listener: listener}

	server.wg.Add(1)

	go server.accept()

	t.Cleanup(func() {
		_ = listener.Close()

		server.wg.Wait()
	})

	return server
}

// Port is the loopback port the fake listens on.
func (s *FakePostgres) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *FakePostgres) accept() {
	defer s.wg.Done()

	var conns sync.WaitGroup

	defer conns.Wait()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		conns.Add(1)

		go func() {
			defer conns.Done()
			defer func() { _ = conn.Close() }()

			_ = servePostgresConn(conn)
		}()
	}
}

func servePostgresConn(conn net.Conn) error {
	reader := bufio.NewReader(conn)

	if err := pgStartup(reader, conn); err != nil {
		return err
	}

	for {
		kind, _, err := readPGMessage(reader)
		if err != nil {
			return err
		}

		switch kind {
		case 'Q':
			if err := writePGMessages(conn, pgMessage('I', nil), pgReadyForQuery()); err != nil {
				return err
			}
		case 'X':
			return nil
		case 'S':
			if err := writePGMessages(conn, pgReadyForQuery()); err != nil {
				return err
			}
		default:
			if err := writePGMessages(conn, pgError("fake postgres supports the simple query protocol only")); err != nil {
				return err
			}
		}
	}
}

// pgStartup declines encryption requests and accepts the startup message
// without authentication.
func pgStartup(reader *bufio.Reader, conn net.Conn) error {
	for {
		var length int32
		if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
			return err
		}

		if length < 8 {
			return errors.New("short startup message")
		}

		body := make([]byte, length-4)
		if _, err := io.ReadFull(reader, body); err != nil {
			return err
		}

		switch binary.BigEndian.Uint32(body[:4]) {
		case pgSSLRequestCode, pgGSSENCRequestCode:
			if _, err := conn.Write([]byte{'N'}); err != nil {
				return err
			}

			continue
		case pgCancelRequestCode:
			return io.EOF
		}

		authOK := binary.BigEndian.AppendUint32(nil, 0)
		backendKey := binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(nil, 1), 1)

		return writePGMessages(
			conn,
			pgMessage('R', authOK),
			pgParameterStatus("server_version", "17.0"),
			pgParameterStatus("client_encoding", "UTF8"),
			pgParameterStatus("standard_conforming_strings", "on"),
			pgParameterStatus("DateStyle", "ISO, MDY"),
			pgParameterStatus("integer_datetimes", "on"),
			pgMessage('K', backendKey),
			pgReadyForQuery(),
		)
	}
}

func readPGMessage(reader *bufio.Reader) (byte, []byte, error) {
	kind, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	var length int32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return 0, nil, err
	}

	if length < 4 {
		return 0, nil, errors.New("malformed message length")
	}

	body := make([]byte, length-4)
	if _, err := io.ReadFull(reader, body); err != nil {
		return 0, nil, err
	}

	return kind, body, nil
}

func pgMessage(kind byte, body []byte) []byte {
	message := make([]byte, 0, 5+len(body))
	message = append(message, kind)
	message = binary.BigEndian.AppendUint32(message, uint32(len(body)+4)) //#nosec G115 -- test messages are tiny
	message = append(message, body...)

	return message
}

func pgParameterStatus(name, value string) []byte {
	body := append([]byte(name), 0)
	body = append(body, value...)
	body = append(body, 0)

	return pgMessage('S', body)
}

func pgReadyForQuery() []byte {
	return pgMessage('Z', []byte{'I'})
}

func pgError(message string) []byte {
	body := make([]byte, 0, len("SERROR\x00C0A000\x00M")+len(message)+2)
	body = append(body, 'S')
	body = append(body, "ERROR"...)
	body = append(body, 0, 'C')
	body = append(body, "0A000"...)
	body = append(body, 0, 'M')
	body = append(body, message...)
	body = append(body, 0, 0)

	return pgMessage('E', body)
}

func writePGMessages(conn net.Conn, messages ...[]byte) error {
	for _, message := range messages {
		if _, err := conn.Write(message); err != nil {
			return err
		}
	}

	return nil
}
