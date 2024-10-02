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
CREATE INDEX IF NOT EXISTS idx_v4 ON "casbin_lerian_rule" ("v4");
CREATE INDEX IF NOT EXISTS idx_v5 ON "casbin_lerian_rule" ("v5");
CREATE INDEX IF NOT EXISTS idx_ptype ON "casbin_lerian_rule" ("ptype");
CREATE INDEX IF NOT EXISTS idx_v0 ON "casbin_lerian_rule" ("v0");
CREATE INDEX IF NOT EXISTS idx_v1 ON "casbin_lerian_rule" ("v1");
CREATE INDEX IF NOT EXISTS idx_v2 ON "casbin_lerian_rule" ("v2");
CREATE INDEX IF NOT EXISTS idx_v3 ON "casbin_lerian_rule" ("v3");

-- Insert the default group and policy
INSERT INTO "casbin_lerian_rule" ("id", "ptype", "v0", "v1", "v2", "v3", "v4", "v5") VALUES
(1,	'g',	'user_john',	'admin_role',	'app-midaz',	'',	'',	''),
(2,	'g',	'user_kate',	'admin_role',	'app-midaz',	'',	'',	''),
(3,	'g',	'user_lisa',	'admin_role',	'app-midaz',	'',	'',	''),
(4,	'g',	'user_john',	'developer_role',	'app-midaz',	'',	'',	''),
(5,	'g',	'user_kate',	'developer_role',	'app-midaz',	'',	'',	''),
(6,	'g',	'user_bob',	'developer_role',	'app-midaz',	'',	'',	''),
(7,	'g',	'user_mike',	'user_role',	'app-midaz',	'',	'',	''),
(8,	'p',	'admin_role',	'app-midaz',	'*',	'*',	'',	''),
(9,	'p',	'developer_role',	'app-midaz',	'*',	'POST',	'',	''),
(10,	'p',	'developer_role',	'app-midaz',	'*',	'GET',	'',	''),
(12,	'p',	'developer_role',	'app-midaz',	'*',	'PATCH',	'',	''),
(11,	'p',	'developer_role',	'app-midaz',	'*',	'PUT',	'',	''),
(13,	'p',	'user_role',	'app-midaz',	'*',	'GET',	'',	'');
