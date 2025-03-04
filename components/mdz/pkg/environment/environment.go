package environment

const CLIVersion = "Mdz CLI "

var (
	ClientID          string
	ClientSecret      string
	URLAPIAuth        string
	URLAPIOnboarding  string
	URLAPITransaction string
	Version           string
)

type Env struct {
	ClientID          string
	ClientSecret      string
	URLAPIAuth        string
	URLAPIOnboarding  string
	URLAPITransaction string
	Version           string
}

func New() *Env {
	return &Env{
		ClientID:          ClientID,
		ClientSecret:      ClientSecret,
		URLAPIAuth:        URLAPIAuth,
		URLAPIOnboarding:  URLAPIOnboarding,
		URLAPITransaction: URLAPITransaction,
		Version:           CLIVersion + Version,
	}
}
