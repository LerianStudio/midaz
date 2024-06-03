package pkg

import (
	"context"
	"net/http"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
)

// GetAuthRelyingParty returns a NewRelyingPartyOIDC that creates an (OIDC)
func GetAuthRelyingParty(ctx context.Context, httpClient *http.Client, membershipURI string) (rp.RelyingParty, error) {
	return rp.NewRelyingPartyOIDC(ctx, membershipURI, AuthClient, "",
		"", []string{"openid", "email", "offline_access", "supertoken", "accesses"}, rp.WithHTTPClient(httpClient))
}
