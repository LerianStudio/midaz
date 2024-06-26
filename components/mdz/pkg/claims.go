package pkg

type stackClaim struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}
type organizationClaim struct {
	ID          string       `json:"id"`
	DisplayName string       `json:"displayName"`
	Stacks      []stackClaim `json:"stacks"`
}
type (
	organizationsClaim []organizationClaim
	userClaims         struct {
		Email   string             `json:"email"`
		Subject string             `json:"sub"`
		Org     organizationsClaim `json:"org"`
	}
)
