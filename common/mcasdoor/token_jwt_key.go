package mcasdoor

import (
	_ "embed"
	"errors"
)

//go:embed certificates/token_jwt_key.pem
var jwtPKCertificate []byte

func LoadCertificate() ([]byte, error) {
	if len(jwtPKCertificate) == 0 {
		return nil, errors.New("public key certificate is empty")
	}
	return jwtPKCertificate, nil
}
