package itestkit

import "github.com/testcontainers/testcontainers-go"

type Customizer = testcontainers.ContainerCustomizer

type CustomizerFunc func(req *testcontainers.GenericContainerRequest) error

func (f CustomizerFunc) Customize(req *testcontainers.GenericContainerRequest) error {
	if f == nil {
		return nil
	}

	return f(req)
}

func MergeCustomizers(lists ...[]Customizer) []Customizer {
	var out []Customizer

	for _, l := range lists {
		if len(l) == 0 {
			continue
		}

		out = append(out, l...)
	}

	return out
}
