package http

import (
	"net/http"
	"strings"
)

// IPAddrFromRemoteAddr removes port information from string.
func IPAddrFromRemoteAddr(s string) string {
	idx := strings.LastIndex(s, ":")
	if idx == -1 {
		return s
	}

	return s[:idx]
}

// GetRemoteAddress returns IP address of the client making the request.
// It checks for X-Real-Ip or X-Forwarded-For headers which is used by Proxies.
func GetRemoteAddress(r *http.Request) string {
	realIP := r.Header.Get(headerRealIP)
	forwardedFor := r.Header.Get(headerForwardedFor)

	if realIP == "" && forwardedFor == "" {
		return IPAddrFromRemoteAddr(r.RemoteAddr)
	}

	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}

		return parts[0]
	}

	return realIP
}
