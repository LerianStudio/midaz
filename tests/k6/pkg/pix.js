/**
 * PIX Indirect BTG API Package
 *
 * Re-exports all PIX API modules for convenient imports.
 * Following existing codebase pattern from /pkg/midaz.js
 *
 * Usage:
 *   import * as pix from './pkg/pix.js';
 *   pix.collection.create(...);
 *   pix.transfer.initiate(...);
 *   pix.refund.create(...);
 *   pix.dict.create(...);
 */

export * as collection from '../apis/pix/collection.js';
export * as transfer from '../apis/pix/transfer.js';
export * as refund from '../apis/pix/refund.js';
export * as dict from '../apis/pix/dict.js';
