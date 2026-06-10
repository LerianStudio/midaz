// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

// BlockProperty describes a single configurable property of a template block.
type BlockProperty struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Values   []string `json:"values,omitempty"`
	Required bool     `json:"required"`
} // @name BlockProperty

// BlockDefinition describes a template block type available for the visual builder.
type BlockDefinition struct {
	Type            string          `json:"type"`
	Label           string          `json:"label"`
	Category        string          `json:"category"`
	AcceptsChildren bool            `json:"acceptsChildren"`
	Properties      []BlockProperty `json:"properties,omitempty"`
} // @name BlockDefinition

// FilterDefinition describes a Pongo2 filter available for the visual builder.
type FilterDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Args        []string `json:"args"`
	Example     string   `json:"example"`
} // @name FilterDefinition

// BlocksConfigResponse is the API response for the blocks-config endpoint.
type BlocksConfigResponse struct {
	Blocks []BlockDefinition `json:"blocks"`
} // @name BlocksConfigResponse

// FiltersResponse is the API response for the filters endpoint.
type FiltersResponse struct {
	Filters []FilterDefinition `json:"filters"`
} // @name FiltersResponse

// GetBlockDefinitions returns all available block definitions for the template builder.
func GetBlockDefinitions() []BlockDefinition {
	return []BlockDefinition{
		{
			Type:     "text",
			Label:    "Texto",
			Category: "basic",
		},
		{
			Type:     "variable",
			Label:    "Variavel",
			Category: "basic",
		},
		{
			Type:            "loop",
			Label:           "Loop",
			Category:        "control",
			AcceptsChildren: true,
		},
		{
			Type:            "conditional",
			Label:           "Conditional",
			Category:        "control",
			AcceptsChildren: true,
		},
		{
			Type:     "aggregation",
			Label:    "Agregacao",
			Category: "data",
		},
		{
			Type:     "calculation",
			Label:    "Calculo",
			Category: "data",
		},
		{
			Type:     "date_time",
			Label:    "Data/Hora",
			Category: "data",
		},
		{
			Type:     "counter",
			Label:    "Contador",
			Category: "dimp",
			Properties: []BlockProperty{
				{
					Name:     "counterMode",
					Type:     "enum",
					Values:   []string{"increment", "show"},
					Required: true,
				},
				{
					Name:     "counterNames",
					Type:     "string[]",
					Required: true,
				},
			},
		},
		{
			Type:     "comment",
			Label:    "Comentario",
			Category: "basic",
		},
		{
			Type:            "section",
			Label:           "Secao",
			Category:        "layout",
			AcceptsChildren: true,
		},
		{
			Type:            "with",
			Label:           "With (Atribuicao)",
			Category:        "control",
			AcceptsChildren: true,
		},
		{
			Type:     "expression",
			Label:    "Expressao",
			Category: "data",
		},
		{
			Type:     "custom_tag",
			Label:    "Tag Customizada",
			Category: "advanced",
			Properties: []BlockProperty{
				{
					Name:     "tagName",
					Type:     "enum",
					Values:   []string{"sum_by", "count_by", "avg_by", "min_by", "max_by", "calc", "last_item_by_group"},
					Required: true,
				},
				{
					Name:     "tagArgs",
					Type:     "string",
					Required: true,
				},
			},
		},
	}
}

// GetFilterDefinitions returns all available filter definitions for the template builder.
func GetFilterDefinitions() []FilterDefinition {
	return []FilterDefinition{
		{
			Name:        "percent_of",
			Description: "Calcula percentual relativo a um valor",
			Args:        []string{"denominator"},
			Example:     `{{ value|percent_of:total }}`,
		},
		{
			Name:        "slice_str",
			Description: "Extrai substring por posicao",
			Args:        []string{"start:end"},
			Example:     `{{ text|slice_str:"0:5" }}`,
		},
		{
			Name:        "strip_zeros",
			Description: "Remove zeros a direita de numeros",
			Args:        []string{},
			Example:     `{{ value|strip_zeros }}`,
		},
		{
			Name:        "replace",
			Description: "Substitui texto",
			Args:        []string{"search:replacement"},
			Example:     `{{ val|replace:".:," }}`,
		},
		{
			Name:        "where",
			Description: "Filtra array por campo",
			Args:        []string{"field:value"},
			Example:     `{{ items|where:"state:SP" }}`,
		},
		{
			Name:        "sum",
			Description: "Soma campo de array",
			Args:        []string{"field"},
			Example:     `{{ items|sum:"value" }}`,
		},
		{
			Name:        "count",
			Description: "Conta elementos filtrados por campo",
			Args:        []string{"field:value"},
			Example:     `{{ items|count:"field:value" }}`,
		},
		{
			Name:        "floatformat",
			Description: "Formata numero com casas decimais",
			Args:        []string{"decimal_places"},
			Example:     `{{ value|floatformat:"2" }}`,
		},
		{
			Name:        "length",
			Description: "Retorna tamanho de uma lista ou string",
			Args:        []string{},
			Example:     `{{ items|length }}`,
		},
		{
			Name:        "date",
			Description: "Formata data com layout Go",
			Args:        []string{"format"},
			Example:     `{{ created_at|date:"02/01/2006" }}`,
		},
	}
}
