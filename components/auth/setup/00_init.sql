-- Create the sequence used for the primary key
CREATE SEQUENCE IF NOT EXISTS "casbin_lerian_rule_id_seq";

-- Create the table used by casbin to enforce permissions
CREATE TABLE IF NOT EXISTS "casbin_lerian_rule" (
    "id" BIGINT PRIMARY KEY DEFAULT nextval('casbin_lerian_rule_id_seq'),
    "ptype" CHARACTER VARYING(100),
    "v0" CHARACTER VARYING(100),
    "v1" CHARACTER VARYING(100),
    "v2" CHARACTER VARYING(100),
    "v3" CHARACTER VARYING(100),
    "v4" CHARACTER VARYING(100),
    "v5" CHARACTER VARYING(100)
);

-- Create indexes for the table
CREATE INDEX IF NOT EXISTS idx_ptype ON "casbin_lerian_rule" ("ptype");
CREATE INDEX IF NOT EXISTS idx_v0 ON "casbin_lerian_rule" ("v0");
CREATE INDEX IF NOT EXISTS idx_v1 ON "casbin_lerian_rule" ("v1");
CREATE INDEX IF NOT EXISTS idx_v2 ON "casbin_lerian_rule" ("v2");
CREATE INDEX IF NOT EXISTS idx_v3 ON "casbin_lerian_rule" ("v3");
CREATE INDEX IF NOT EXISTS idx_v4 ON "casbin_lerian_rule" ("v4");
CREATE INDEX IF NOT EXISTS idx_v5 ON "casbin_lerian_rule" ("v5");

-- Insert the default group and policy
INSERT INTO "casbin_lerian_rule" ("ptype", "v0", "v1", "v2", "v3", "v4", "v5") VALUES
('g',	'user_john',	'admin_role',	'',	'',	'',	''),
('g',	'user_kate',	'admin_role',	'',	'',	'',	''),
('g',	'user_lisa',	'admin_role',	'',	'',	'',	''),
('g',	'user_john',	'developer_role',	'',	'',	'',	''),
('g',	'user_john',	'grpc_role',	'',	'',	'',	''),
('g',	'user_kate',	'developer_role',	'',	'',	'',	''),
('g',	'user_bob',	'developer_role',	'',	'',	'',	''),
('g',	'user_bob',	'grpc_role',	'',	'',	'',	''),
('g',	'user_mike',	'user_role',	'',	'',	'',	''),
('p',	'admin_role',	'*',	'*',	'',	'',	''),
('p',	'developer_role',	'*',	'POST',	'',	'',	''),
('p',	'developer_role',	'*',	'GET',	'',	'',	''),
('p',	'developer_role',	'*',	'PUT',	'',	'',	''),
('p',	'developer_role',	'*',	'PATCH',	'',	'',	''),
('p',	'user_role',	'*',	'GET',	'',	'',	''),
('p',	'grpc_role',	'account.AccountProto',	'*',	'',	'',	'');
