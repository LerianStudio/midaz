package environment

var (
	ClientID     string
	ClientSecret string
	URLAPIAuth   string
	URLAPILedger string
)

type Env struct {
	ClientID     string
	ClientSecret string
	URLAPIAuth   string
	URLAPILedger string
}

func New() *Env {
	return &Env{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		URLAPIAuth:   URLAPIAuth,
		URLAPILedger: URLAPILedger,
	}
}
