package login

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
)

var (
	srvCallBackCtx    context.Context
	srvCallBackCancel context.CancelFunc
)

func initializeContext() {
	srvCallBackCtx, srvCallBackCancel = context.WithCancel(context.Background())
}

type browser struct {
	Err error
}

func (l *factoryLogin) browserLogin() {
	clientID := "9670e0ca55a29a466d31"
	redirectURI := "http://localhost:9000/callback"
	state := "random_state"

	authURL := fmt.Sprintf("%s/login/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=openid&state=%s",
		l.factory.Env.URLAPIAuth, clientID, url.QueryEscape(redirectURI), state)

	err := l.openBrowser(authURL)

	if err != nil {
		l.browser.Err = err
		output.Printf(l.factory.IOStreams.Err, err.Error())

		return
	}

	http.HandleFunc("/callback", l.callbackHandler)
	initializeContext()

	server := http.Server{Addr: ":9000", ReadHeaderTimeout: 5 * time.Second}

	go func() {
		output.Printf(l.factory.IOStreams.Out, "Server running on http://localhost:9000...")

		err := server.ListenAndServe()

		if err != http.ErrServerClosed {
			l.browser.Err = err
			output.Printf(l.factory.IOStreams.Out,
				"Error while serving server for browser login "+err.Error())

			return
		}
	}()

	<-srvCallBackCtx.Done() // wait for the signal to gracefully shutdown the server

	err = server.Shutdown(context.Background())
	if err != nil {
		l.browser.Err = err
		output.Printf(l.factory.IOStreams.Err, err.Error())

		return
	}
}

// openBrowser to open the browser in the operating system
func (l *factoryLogin) openBrowser(u string) error {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", u).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		err = exec.Command("open", u).Start()
	default:
		err = errors.New("unsupported platform")
	}

	if err != nil {
		return errors.New("opening the browser: " + err.Error())
	}

	output.Printf(l.factory.IOStreams.Out, "Wait Authenticated via browser...")

	return nil
}

// callbackHandler handles the callback and exchanges the code for the token
func (l *factoryLogin) callbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")

	if code == "" {
		http.Error(w, "Authorization code not found", http.StatusBadRequest)
		return
	}

	token, err := l.auth.ExchangeToken(code)

	if err != nil {
		l.browser.Err = err

		http.Error(w,
			"Failed to exchange authorization code for access token. Please try "+
				"again or contact support. :(",
			http.StatusInternalServerError)
		output.Printf(l.factory.IOStreams.Err, err.Error())

		return
	}

	if token != nil {
		l.token = token.AccessToken
	}

	htmlResponse := `
		<!DOCTYPE html>

		<html lang="en">
		<head>

			<meta charset="UTF-8">

			<meta name="viewport" content="width=device-width, initial-scale=1.0">

			<link rel="icon" type="image/png" sizes="32x32" href="https://avatars.githubusercontent.com/u/148895005?v=4">
			<title>Midaz</title>
			<style>
				body {
					display: flex;
					flex-direction: column;
					justify-content: center;
					align-items: center;
					height: 100vh;
					margin: 0;
					font-family: Arial, sans-serif;
					background-color: #f4f4f4;
				}
				.container {
					text-align: center;
				}
				.logo {
					width: 150px;
				}
				.text {
					color: #000;
					font-size: 12px;
					margin-top: 20px;
				}
				.footer {
					position: fixed;
					bottom: 10px;
					text-align: center;
					width: 100%;
					font-size: 14px;
					color: #888;
				}
				.footer a {
					color: #000;
					text-decoration: none;
				}
			</style>
		</head>
		<body>

			<div class="container">

				<img src="https://avatars.githubusercontent.com/u/148895005?v=4" alt="Logo" class="logo">

				<div class="text">Authenticated, you can now close this page and return to your terminal</div>
			</div>

			<div class="footer">

				<p>Made with <span style="color: #e25555;">&#x2764;</span> by <a href="https://github.com/maxwelbm" style="color: #000; text-decoration: none;">maxwelbm</a></p>

				<p>&copy; 2024 <a href="https://github.com/LerianStudio/midaz", >Midaz</a>. Licensed under the <a href="https://www.apache.org/licenses/LICENSE-2.0" target="_blank" style="color: #000;">Apache-2.0 License</a>. All rights reserved.</p>
			</div>
		</body>
		</html>`

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write([]byte(htmlResponse))
	if err != nil {
		l.browser.Err = err

		output.Printf(l.factory.IOStreams.Err, err.Error())

		http.Error(w, "Failed to render HTML", http.StatusInternalServerError)
	}

	srvCallBackCancel()
}
