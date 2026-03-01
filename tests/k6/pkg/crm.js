/**
 * CRM API Package
 *
 * Re-exports all CRM API modules for convenient imports.
 * Following existing codebase pattern from /pkg/midaz.js
 *
 * Usage:
 *   import * as crm from './pkg/crm.js';
 *   crm.holder.create(...);
 *   crm.alias.create(...);
 */

export * as holder from '../apis/crm/holder.js';
export * as alias from '../apis/crm/alias.js';
