package http

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// ServeReverseProxy serves a reverse proxy for a given url.
func ServeReverseProxy(target string, res http.ResponseWriter, req *http.Request) {
	targetURL, err := url.Parse(target)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Update the headers to allow for SSL redirection
	req.URL.Host = targetURL.Host
	req.URL.Scheme = targetURL.Scheme
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = targetURL.Host

	proxy.ServeHTTP(res, req)
}
