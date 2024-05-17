package pkg

import (
	"net/http"

	"github.com/zitadel/oidc/v2/pkg/client/rp"
)

// GetAuthRelyingParty returns a NewRelyingPartyOIDC that creates an (OIDC)
func GetAuthRelyingParty(httpClient *http.Client, membershipURI string) (rp.RelyingParty, error) {
	return rp.NewRelyingPartyOIDC(membershipURI, AuthClient, "",
		"", []string{"openid", "email", "offline_access", "supertoken", "accesses"}, rp.WithHTTPClient(httpClient))
}
