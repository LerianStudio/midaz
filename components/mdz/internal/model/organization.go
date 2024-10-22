package model

type Organization struct {
	LegalName       string   `json:"legalName"`
	DoingBusinessAs string   `json:"doingBusinessAs"`
	LegalDocument   string   `json:"legalDocument"`
	Status          Status   `json:"status"`
	Address         Address  `json:"address"`
	Metadata        Metadata `json:"metadata"`
}

type Status struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type Address struct {
	Line1   string `json:"line1"`
	Line2   string `json:"line2"`
	ZipCode string `json:"zipCode"`
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
}

type Metadata struct {
	Chave   string  `json:"chave"`
	Bitcoin string  `json:"bitcoinn"`
	Boolean bool    `json:"boolean"`
	Double  float64 `json:"double"`
	Int     int     `json:"int"`
}

type OrganizationResponse struct {
	ID                   string   `json:"id"`
	ParentOrganizationID *string  `json:"parentOrganizationId"`
	LegalName            string   `json:"legalName"`
	DoingBusinessAs      string   `json:"doingBusinessAs"`
	LegalDocument        string   `json:"legalDocument"`
	Address              Address  `json:"address"`
	Status               Status   `json:"status"`
	CreatedAt            string   `json:"createdAt"`
	UpdatedAt            string   `json:"updatedAt"`
	DeletedAt            *string  `json:"deletedAt"`
	Metadata             Metadata `json:"metadata"`
}
