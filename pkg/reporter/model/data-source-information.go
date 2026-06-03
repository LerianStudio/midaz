// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

// DataSourceInformation is a struct designed to encapsulate data source information of database connected on plugin
//
// swagger:model DataSourceInformation
//
//	@Description	DataSourceInformation is the data source information of database connected on plugin
type DataSourceInformation struct {
	Id           string `json:"id" example:"midaz_onboarding"`
	ExternalName string `json:"externalName" example:"onboarding"`
	Type         string `json:"type" example:"postgresql"`
} //	@name	DataSourceInformation

// DataSourceDetails is a struct designed to encapsulate data source details of database connected on plugin
//
// swagger:model DataSourceDetails
//
//	@Description	DataSourceDetails is the data source details of database
type DataSourceDetails struct {
	Id           string         `json:"id" example:"midaz_onboarding"`
	ExternalName string         `json:"externalName" example:"onboarding"`
	Type         string         `json:"type" example:"postgresql"`
	Tables       []TableDetails `json:"tables"`
} //	@name	DataSourceDetails

// TableDetails is a struct designed to encapsulate the table information with the columns and name
//
// swagger:model TableDetails
//
//	@Description	TableDetails is the struct of table information
type TableDetails struct {
	Name   string   `json:"name" example:"account"`
	Fields []string `json:"fields" example:"['id', 'name', 'parent_account_id']"`
} //	@name	TableDetails
