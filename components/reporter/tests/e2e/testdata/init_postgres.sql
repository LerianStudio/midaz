-- Reporter E2E Test: PostgreSQL seed data
-- Schema: midaz_onboarding (public schema on database midaz_onboarding)

CREATE TABLE IF NOT EXISTS organization (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    legal_name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    legal_document VARCHAR(50) NOT NULL,
    country VARCHAR(10) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE TABLE IF NOT EXISTS ledger (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organization(id),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS account (
    id UUID PRIMARY KEY,
    ledger_id UUID NOT NULL REFERENCES ledger(id),
    organization_id UUID NOT NULL REFERENCES organization(id),
    name VARCHAR(255) NOT NULL,
    alias VARCHAR(255),
    type VARCHAR(50) NOT NULL,
    asset_code VARCHAR(10) NOT NULL,
    balance NUMERIC(18,2) NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Organizations: 10 rows with varied statuses and dates
INSERT INTO organization (id, name, legal_name, status, legal_document, country, created_at, updated_at) VALUES
('a0000000-0000-0000-0000-000000000001', 'Acme Corp', 'Acme Corporation Ltd', 'active', '12345678000101', 'BR', '2025-01-15T10:00:00Z', '2025-01-15T10:00:00Z'),
('a0000000-0000-0000-0000-000000000002', 'Beta Inc', 'Beta Incorporated', 'active', '23456789000102', 'BR', '2025-02-20T11:00:00Z', '2025-02-20T11:00:00Z'),
('a0000000-0000-0000-0000-000000000003', 'Gamma LLC', 'Gamma Limited Liability', 'suspended', '34567890000103', 'US', '2025-03-10T12:00:00Z', '2025-03-10T12:00:00Z'),
('a0000000-0000-0000-0000-000000000004', 'Delta SA', 'Delta Sociedade Anonima', 'active', '45678901000104', 'BR', '2025-04-05T09:00:00Z', '2025-04-05T09:00:00Z'),
('a0000000-0000-0000-0000-000000000005', 'Epsilon Ltd', 'Epsilon Limited', 'inactive', '56789012000105', 'UK', '2025-05-01T08:00:00Z', '2025-05-01T08:00:00Z'),
('a0000000-0000-0000-0000-000000000006', 'Zeta Corp', 'Zeta Corporation', 'active', '67890123000106', 'BR', '2025-06-15T14:00:00Z', '2025-06-15T14:00:00Z'),
('a0000000-0000-0000-0000-000000000007', 'Eta Group', 'Eta Group Holdings', 'pending', '78901234000107', 'BR', '2025-07-20T15:00:00Z', '2025-07-20T15:00:00Z'),
('a0000000-0000-0000-0000-000000000008', 'Theta Inc', 'Theta Incorporated', 'active', '89012345000108', 'US', '2025-08-10T16:00:00Z', '2025-08-10T16:00:00Z'),
('a0000000-0000-0000-0000-000000000009', 'Iota SA', 'Iota Sociedade Anonima', 'suspended', '90123456000109', 'BR', '2025-09-05T17:00:00Z', '2025-09-05T17:00:00Z'),
('a0000000-0000-0000-0000-000000000010', 'Kappa Ltd', 'Kappa Limited', 'active', '01234567000110', 'UK', '2025-10-01T18:00:00Z', '2025-10-01T18:00:00Z');

-- Ledgers: 7 rows linked to 5 organizations
INSERT INTO ledger (id, organization_id, name, status, created_at) VALUES
('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'Main Ledger', 'active', '2025-01-20T10:00:00Z'),
('b0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'Secondary Ledger', 'active', '2025-02-01T10:00:00Z'),
('b0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000002', 'Primary Ledger', 'active', '2025-02-25T10:00:00Z'),
('b0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000003', 'Operations Ledger', 'suspended', '2025-03-15T10:00:00Z'),
('b0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000004', 'Treasury Ledger', 'active', '2025-04-10T10:00:00Z'),
('b0000000-0000-0000-0000-000000000006', 'a0000000-0000-0000-0000-000000000005', 'Archive Ledger', 'inactive', '2025-05-05T10:00:00Z'),
('b0000000-0000-0000-0000-000000000007', 'a0000000-0000-0000-0000-000000000006', 'Revenue Ledger', 'active', '2025-06-20T10:00:00Z');

-- Accounts: 7 rows with varied types and balances
INSERT INTO account (id, ledger_id, organization_id, name, alias, type, asset_code, balance, status, created_at) VALUES
('c0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'Operating Account', 'op-acme', 'deposit', 'BRL', 250000.00, 'active', '2025-01-25T10:00:00Z'),
('c0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'Savings Account', 'sav-acme', 'savings', 'BRL', 500000.00, 'active', '2025-02-05T10:00:00Z'),
('c0000000-0000-0000-0000-000000000003', 'b0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000002', 'Expense Account', 'exp-beta', 'expense', 'USD', 75000.50, 'active', '2025-03-01T10:00:00Z'),
('c0000000-0000-0000-0000-000000000004', 'b0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000003', 'Investment Account', 'inv-gamma', 'investment', 'BRL', 750000.00, 'suspended', '2025-03-20T10:00:00Z'),
('c0000000-0000-0000-0000-000000000005', 'b0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000004', 'Checking Account', 'chk-delta', 'deposit', 'BRL', 150000.00, 'active', '2025-04-15T10:00:00Z'),
('c0000000-0000-0000-0000-000000000006', 'b0000000-0000-0000-0000-000000000006', 'a0000000-0000-0000-0000-000000000005', 'Reserve Account', 'res-epsilon', 'savings', 'GBP', 25000.00, 'inactive', '2025-05-10T10:00:00Z'),
('c0000000-0000-0000-0000-000000000007', 'b0000000-0000-0000-0000-000000000007', 'a0000000-0000-0000-0000-000000000006', 'Revenue Account', 'rev-zeta', 'deposit', 'BRL', 350000.00, 'active', '2025-06-25T10:00:00Z');

-- Reporter E2E Test: PostgreSQL seed data
-- Schema: midaz_transaction (public schema on database midaz_transaction)

-- Operation routes: used by cadoc-4111 template (midaz_transaction datasource)
CREATE TABLE IF NOT EXISTS operation_route (
    id UUID PRIMARY KEY,
    code VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Operations: transaction records with balances per account and route
CREATE TABLE IF NOT EXISTS operation (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL,
    route UUID NOT NULL REFERENCES operation_route(id),
    available_balance_after NUMERIC(18,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Operation routes: 2 CADOC 4111 codes
INSERT INTO operation_route (id, code, created_at) VALUES
('d0000000-0000-0000-0000-000000000001', '4111001', '2025-01-01T10:00:00Z'),
('d0000000-0000-0000-0000-000000000002', '4111002', '2025-01-01T10:00:00Z');

-- Operations: multiple entries per route to test aggregation
-- Route 4111001: two operations for same account (last_item_by_group keeps latest)
-- Route 4111002: one operation
INSERT INTO operation (id, account_id, route, available_balance_after, created_at) VALUES
('e0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 100000.00, '2025-06-01T10:00:00Z'),
('e0000000-0000-0000-0000-000000000002', 'c0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 150000.00, '2025-06-02T10:00:00Z'),
('e0000000-0000-0000-0000-000000000003', 'c0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000002', 500000.00, '2025-06-01T10:00:00Z');
