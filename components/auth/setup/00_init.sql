-- Create the sequence used for the primary key
CREATE SEQUENCE IF NOT EXISTS "casbin_lerian_enforcer_rule_id_seq";

-- Create the table used by casbin to enforce permissions
CREATE TABLE IF NOT EXISTS "casbin_lerian_enforcer_rule" (
    "id" BIGINT PRIMARY KEY DEFAULT nextval('casbin_lerian_enforcer_rule_id_seq'),
    "ptype" CHARACTER VARYING(100),
    "v0" CHARACTER VARYING(100),
    "v1" CHARACTER VARYING(100),
    "v2" CHARACTER VARYING(100),
    "v3" CHARACTER VARYING(100),
    "v4" CHARACTER VARYING(100),
    "v5" CHARACTER VARYING(100)
);

-- Create indexes for the table
CREATE INDEX IF NOT EXISTS idx_ptype ON "casbin_lerian_enforcer_rule" ("ptype");
CREATE INDEX IF NOT EXISTS idx_v0 ON "casbin_lerian_enforcer_rule" ("v0");
CREATE INDEX IF NOT EXISTS idx_v1 ON "casbin_lerian_enforcer_rule" ("v1");
CREATE INDEX IF NOT EXISTS idx_v2 ON "casbin_lerian_enforcer_rule" ("v2");
CREATE INDEX IF NOT EXISTS idx_v3 ON "casbin_lerian_enforcer_rule" ("v3");
CREATE INDEX IF NOT EXISTS idx_v4 ON "casbin_lerian_enforcer_rule" ("v4");
CREATE INDEX IF NOT EXISTS idx_v5 ON "casbin_lerian_enforcer_rule" ("v5");

-- Insert the default group and policy
INSERT INTO "casbin_lerian_enforcer_rule" ("ptype", "v0", "v1", "v2", "v3", "v4", "v5") VALUES
('g',	'user_john',	'admin_role',	'',	'',	'',	''),
('g',	'user_lisa',	'developer_role',	'',	'',	'',	''),
('g',	'user_lisa',	'grpc_role',	'',	'',	'',	''),
('g',	'user_bob',	'grpc_role',	'',	'',	'',	''),
('g',	'user_mike',	'user_role',	'',	'',	'',	''),
('g',	'user_kate',	'user_role',	'',	'',	'',	''),
('g',	'user_jane',	'auditor_role',	'',	'',	'',	''),
('p',	'admin_role',	'*',	'*',	'',	'',	''),
('p',	'developer_role',	'organization',	'post',	'',	'',	''),
('p',	'developer_role',	'organization',	'get',	'',	'',	''),
('p',	'developer_role',	'organization',	'patch',	'',	'',	''),
('p',	'developer_role',	'organization',	'put',	'',	'',	''),
('p',	'developer_role',	'ledger',	'post',	'',	'',	''),
('p',	'developer_role',	'ledger',	'get',	'',	'',	''),
('p',	'developer_role',	'ledger',	'patch',	'',	'',	''),
('p',	'developer_role',	'ledger',	'put',	'',	'',	''),
('p',	'developer_role',	'asset',	'post',	'',	'',	''),
('p',	'developer_role',	'asset',	'get',	'',	'',	''),
('p',	'developer_role',	'asset',	'patch',	'',	'',	''),
('p',	'developer_role',	'asset',	'put',	'',	'',	''),
('p',	'developer_role',	'portfolio',	'post',	'',	'',	''),
('p',	'developer_role',	'portfolio',	'get',	'',	'',	''),
('p',	'developer_role',	'portfolio',	'patch',	'',	'',	''),
('p',	'developer_role',	'portfolio',	'put',	'',	'',	''),
('p',	'developer_role',	'cluster',	'post',	'',	'',	''),
('p',	'developer_role',	'cluster',	'get',	'',	'',	''),
('p',	'developer_role',	'cluster',	'patch',	'',	'',	''),
('p',	'developer_role',	'cluster',	'put',	'',	'',	''),
('p',	'developer_role',	'account',	'post',	'',	'',	''),
('p',	'developer_role',	'account',	'get',	'',	'',	''),
('p',	'developer_role',	'account',	'patch',	'',	'',	''),
('p',	'developer_role',	'account',	'put',	'',	'',	''),
('p',	'developer_role',	'transaction',	'post',	'',	'',	''),
('p',	'developer_role',	'transaction',	'get',	'',	'',	''),
('p',	'developer_role',	'transaction',	'patch',	'',	'',	''),
('p',	'developer_role',	'transaction',	'put',	'',	'',	''),
('p',	'developer_role',	'operation',	'post',	'',	'',	''),
('p',	'developer_role',	'operation',	'get',	'',	'',	''),
('p',	'developer_role',	'operation',	'patch',	'',	'',	''),
('p',	'developer_role',	'operation',	'put',	'',	'',	''),
('p',	'developer_role',	'asset-rate',	'put',	'',	'',	''),
('p',	'developer_role',	'asset-rate',	'get',	'',	'',	''),
('p',	'user_role',	'organization',	'get',	'',	'',	''),
('p',	'user_role',	'ledger',	'get',	'',	'',	''),
('p',	'user_role',	'asset',	'get',	'',	'',	''),
('p',	'user_role',	'portfolio',	'get',	'',	'',	''),
('p',	'user_role',	'cluster',	'get',	'',	'',	''),
('p',	'user_role',	'account',	'get',	'',	'',	''),
('p',	'user_role',	'transaction',	'get',	'',	'',	''),
('p',	'user_role',	'operation',	'get',	'',	'',	''),
('p',	'user_role',	'asset-rate',	'get',	'',	'',	''),
('p',	'grpc_role',	'account.AccountProto',	'*',	'',	'',	''),
('p',	'auditor_role',	'audit',	'get',	'',	'',	'');
