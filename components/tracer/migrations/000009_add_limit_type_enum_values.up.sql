-- ============================================
-- Migration: 000009_add_limit_type_enum_values
-- Description: Add WEEKLY and CUSTOM values to limit_type_enum
-- Date: 2026-03-10
-- ============================================
-- Note: Adding enum values must be in a separate migration from column changes
-- PostgreSQL requires ADD VALUE to be the only statement in a transaction when
-- NOT using IF NOT EXISTS (prior to PG 10), and golang-migrate handles each file
-- as a separate transaction.

-- Add WEEKLY type for limits that reset every Monday at 00:00 UTC
ALTER TYPE limit_type_enum ADD VALUE IF NOT EXISTS 'WEEKLY';

-- Add CUSTOM type for limits with user-defined periods
ALTER TYPE limit_type_enum ADD VALUE IF NOT EXISTS 'CUSTOM';
