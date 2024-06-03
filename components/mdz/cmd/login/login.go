package login

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg"
	"github.com/pkg/browser"
	"github.com/pterm/pterm"

	"github.com/spf13/cobra"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// LogIn func that can log in on midaz
func LogIn(ctx context.Context, dialog Dialog, relyingParty rp.RelyingParty) (*oidc.AccessTokenResponse, error) {
	deviceCode, err := rp.DeviceAuthorization(ctx, relyingParty.OAuthConfig().Scopes, relyingParty, nil)
	if err != nil {
		return nil, err
	}

	uri, err := url.Parse(deviceCode.VerificationURI)
	if err != nil {
		panic(err)
	}

	query := uri.Query()
	query.Set("user_code", deviceCode.UserCode)
	uri.RawQuery = query.Encode()

	if err := browser.OpenURL(uri.String()); err != nil {
		if !errors.Is(err, pkg.ErrOpenningBrowser) {
			return nil, err
		}

		fmt.Println("No browser detected")
	}

	dialog.DisplayURIAndCode(deviceCode.VerificationURI, deviceCode.UserCode)

	return rp.DeviceAccessToken(ctx, deviceCode.DeviceCode, time.Duration(deviceCode.Interval)*time.Second, relyingParty)
}

// Dialog provides an interface for DisplayURIAndCode
type Dialog interface {
	DisplayURIAndCode(uri, code string)
}

// DialogFn is a func that implements uri and code
type DialogFn func(uri, code string)

// DisplayURIAndCode is a func that return a type DialogFn
func (fn DialogFn) DisplayURIAndCode(uri, code string) {
	fn(uri, code)
}

// Store is a struct designed to encapsulate payload data.
type Store struct {
	Profile    *pkg.Profile `json:"-"`
	DeviceCode string       `json:"deviceCode"`
	LoginURI   string       `json:"loginUri"`
	BrowserURL string       `json:"browserUrl"`
	Success    bool         `json:"success"`
}

// Controller is a struct to encapsulate a *Store struct.
type Controller struct {
	store *Store
}

// NewDefaultLoginStore is a func that return a struct *Store
func NewDefaultLoginStore() *Store {
	return &Store{
		Profile:    nil,
		DeviceCode: "",
		LoginURI:   "",
		BrowserURL: "",
		Success:    false,
	}
}

// GetStore is a func that return a struct *Store
func (c *Controller) GetStore() *Store {
	return c.store
}

// NewLoginController is a func that return a struct *Controller
func NewLoginController() *Controller {
	return &Controller{
		store: NewDefaultLoginStore(),
	}
}

// Run is a fun that executes Open func and return a Renderable interface
func (c *Controller) Run(cmd *cobra.Command, args []string) (pkg.Renderable, error) {
	cfg, err := pkg.GetConfig(cmd)
	if err != nil {
		return nil, err
	}

	profile := pkg.GetCurrentProfile(cmd, cfg)

	membershipURI, err := cmd.Flags().GetString(pkg.MembershipURIFlag)
	if err != nil {
		return nil, err
	}

	if membershipURI == "" {
		membershipURI = profile.GetMembershipURI()
	}

	relyingParty, err := pkg.GetAuthRelyingParty(pkg.GetHTTPClient(cmd, map[string][]string{}), membershipURI)
	if err != nil {
		return nil, err
	}

	c.store.Profile = profile

	ret, err := LogIn(cmd.Context(), DialogFn(func(uri, code string) {
		c.store.DeviceCode = code
		c.store.LoginURI = uri
		fmt.Println("Link :", fmt.Sprintf("%s?user_code=%s", c.store.LoginURI, c.store.DeviceCode))
	}), relyingParty)
	if err != nil {
		return nil, err
	}

	if ret != nil {
		c.store.Success = true

		profile.UpdateToken(ret)
	}

	profile.SetMembershipURI(membershipURI)

	currentProfileName := pkg.GetCurrentProfileName(cmd, cfg)

	cfg.SetCurrentProfile(currentProfileName, profile)

	return c, cfg.Persist()
}

// Render is a func that show if you are logged
func (c *Controller) Render(cmd *cobra.Command, args []string) error {
	pterm.Success.WithWriter(cmd.OutOrStdout()).Printfln("Logged!")
	return nil
}

// NewCommand is a func that execute some commands
func NewCommand() *cobra.Command {
	return pkg.NewCommand("login",
		pkg.WithStringFlag(pkg.MembershipURIFlag, "", "service url"),
		pkg.WithHiddenFlag(pkg.MembershipURIFlag),
		pkg.WithShortDescription("Login"),
		pkg.WithArgs(cobra.ExactArgs(0)),
		pkg.WithController[*Store](NewLoginController()),
	)
}
