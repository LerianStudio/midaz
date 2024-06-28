package mauth

type AuthClient struct {
	Endpoint     string
	ClientID     string
	ClientSecret string
}

func NewAuthClient() *AuthClient {
	return &AuthClient{
		Endpoint:     "http://localhost:8080/realms/Midaz",
		ClientID:     "midaz",
		ClientSecret: "Tp7iiYdYvkLKA59GL83M2nd1If6eZx3R",
	}
}
