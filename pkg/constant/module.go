// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

const (
	// ModuleOnboarding is the module name for onboarding database schemas.
	ModuleOnboarding = "onboarding"
	// ModuleTransaction is the module name for transaction database schemas.
	ModuleTransaction = "transaction"
	// ModuleCRM is the tenant-manager module name for CRM (holder/alias) database
	// schemas. The value MUST be "crm-api" to match tenant-manager provisioning.
	ModuleCRM = "crm-api"
	// ModuleFees is the tenant-manager module name for fee/billing-package database
	// schemas. The value MUST be "plugin-fees" to match tenant-manager provisioning:
	// the standalone fees service registered its per-tenant Mongo manager under
	// constant.ApplicationName ("plugin-fees") as the tenant-manager SERVICE name
	// with NO WithModule (single-module mode), and its auth/RBAC policies key on
	// the same "plugin-fees" namespace (R9, P4-T22). Renaming this breaks tenant DB
	// resolution and RBAC for tenants already provisioned under "plugin-fees".
	// (Mirrors the CRM crm->crm-api footgun: the embedded name MUST match prod
	// provisioning. See P4-T22 for cross-team confirmation of the provisioning name.)
	ModuleFees = "plugin-fees"
)
