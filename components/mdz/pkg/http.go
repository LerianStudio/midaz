package pkg

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"

	"github.com/TylerBrock/colorjson"
	"github.com/spf13/cobra"
)

// GetHTTPClient creates and returns a new HTTP client configured based on the provided
// cobra.Command flags and default headers. It utilizes the NewHTTPClient function,
// passing in the values of the InsecureTlsFlag and DebugFlag, along with any
// defaultHeaders specified. This allows for the creation of a customized http.Client
// instance tailored to the needs of the application, including support for insecure
// TLS connections and debugging capabilities.
func GetHTTPClient(cmd *cobra.Command, defaultHeaders map[string][]string) *http.Client {
	return NewHTTPClient(
		GetBool(cmd, DebugFlag), // Enables or disables debugging output.
		defaultHeaders,          // Sets default headers for all requests made by the client.
	)
}

// RoundTripperFn is a function type that implements the http.RoundTripper interface.
// It allows any function with the appropriate signature to be used as an http.RoundTripper.
// This is useful for creating custom transport behaviors in an http.Client.
type RoundTripperFn func(req *http.Request) (*http.Response, error)

// RoundTrip executes the RoundTripperFn function, effectively making RoundTripperFn
// an http.RoundTripper. This method allows RoundTripperFn to satisfy the http.RoundTripper
// interface, enabling its use as a custom transport mechanism within an http.Client.
func (fn RoundTripperFn) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func printBody(data []byte) {
	if len(data) == 0 {
		return
	}

	raw := make(map[string]any)

	if err := json.Unmarshal(data, &raw); err == nil {
		f := colorjson.NewFormatter()
		f.Indent = 2

		colorized, err := f.Marshal(raw)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(colorized))
	} else {
		fmt.Println(string(data))
	}
}

func debugRoundTripper(rt http.RoundTripper) RoundTripperFn {
	return func(req *http.Request) (*http.Response, error) {
		data, err := httputil.DumpRequest(req, false)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(data))

		if req.Body != nil {
			data, err = io.ReadAll(req.Body)
			if err != nil {
				panic(err)
			}

			if err := req.Body.Close(); err != nil {
				panic(err)
			}

			req.Body = io.NopCloser(bytes.NewBuffer(data))

			printBody(data)
		}

		rsp, err := rt.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		data, err = httputil.DumpResponse(rsp, false)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(data))

		if rsp.Body != nil {
			data, err = io.ReadAll(rsp.Body)
			if err != nil {
				panic(err)
			}

			if err := rsp.Body.Close(); err != nil {
				panic(err)
			}

			rsp.Body = io.NopCloser(bytes.NewBuffer(data))
			printBody(data)
		}

		return rsp, nil
	}
}

func defaultHeadersRoundTripper(rt http.RoundTripper, headers map[string][]string) RoundTripperFn {
	return func(req *http.Request) (*http.Response, error) {
		for k, v := range headers {
			for _, vv := range v {
				req.Header.Add(k, vv)
			}
		}

		return rt.RoundTrip(req)
	}
}

// NewHTTPClient initializes and returns a new http.Client with customizable behavior.
// It allows for the configuration of TLS insecurity (skipping TLS verification),
// enabling a debug mode for additional logging, and setting default headers for all requests.
//
// Parameters:
//   - insecureTLS: If true, the client will accept any TLS certificate presented by the server
//     and any host name in that certificate. This is useful for testing with self-signed certificates.
//   - debug: If true, wraps the transport in a debugging layer that logs all requests and responses.
//     This is helpful for development and troubleshooting.
//   - defaultHeaders: A map of header names to their values which will be added to every request
//     made by this client. Useful for setting headers like `Authorization` or `User-Agent` that
//     should be included in all requests.
//
// Returns:
// - A pointer to an initialized http.Client configured as specified.
func NewHTTPClient(debug bool, defaultHeaders map[string][]string) *http.Client {
	var transport http.RoundTripper = &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: false,
		},
	}

	if debug {
		transport = debugRoundTripper(transport)
	}

	if len(defaultHeaders) > 0 {
		transport = defaultHeadersRoundTripper(transport, defaultHeaders)
	}

	return &http.Client{
		Transport: transport,
	}
}
