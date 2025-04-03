package environment

const CLIVersion = "Mdz CLI "

var (
	ClientID     string
	ClientSecret string
	URLAPIAuth   string
	URLAPILedger string
	Version      string
)

// \1 represents an entity
type Env struct {
	ClientID     string
	ClientSecret string
	URLAPIAuth   string
	URLAPILedger string
	Version      string
}

// \1 performs an operation
func New() *Env {
	return &Env{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		URLAPIAuth:   URLAPIAuth,
		URLAPILedger: URLAPILedger,
		Version:      CLIVersion + Version,
	}
}
