package itestkit

import (
	"context"
	"fmt"
)

type Infra interface {
	Start(ctx context.Context, env *Env) error
	Terminate(ctx context.Context) error
}

type NamedInfra interface {
	Infra
	InfraKind() string
	InfraName() string
}

func validateUniqueInfraNames(infras []Infra) error {
	seen := map[string]struct{}{}

	for _, inf := range infras {
		ni, ok := inf.(NamedInfra)
		if !ok {
			continue
		}

		kind := ni.InfraKind()

		name := ni.InfraName()
		if name == "" {
			name = "default"
		}

		key := kind + ":" + name
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate infra registration: %s (name=%q)", kind, name)
		}

		seen[key] = struct{}{}
	}

	return nil
}
