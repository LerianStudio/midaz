CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS balance (
  id                                  UUID PRIMARY KEY NOT NULL,
  alias                               TEXT NOT NULL,
  organization_id                     UUID NOT NULL,
  ledger_id                           UUID NOT NULL,
  asset_code                          TEXT NOT NULL,
  available                           NUMERIC NOT NULL DEFAULT 0,
  on_hold                             NUMERIC NOT NULL DEFAULT 0,
  scale                               NUMERIC NOT NULL DEFAULT 0,
  version                             NUMERIC DEFAULT 0,
  created_at                          TIMESTAMP WITH TIME ZONE,
  updated_at                          TIMESTAMP WITH TIME ZONE,
  deleted_at                          TIMESTAMP WITH TIME ZONE
);

INSERT INTO balance VALUES (gen_random_uuid(), '@account1-1_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-2_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-3_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-4_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-5_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-6_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-7_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-8_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-9_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account1-10_BRL', gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-1_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-2_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-3_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-4_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-5_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-6_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-7_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-8_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-9_BRL',  gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());
INSERT INTO balance VALUES (gen_random_uuid(), '@account2-10_BRL', gen_random_uuid(), gen_random_uuid(),'BRL',0,0,0,0,NOW(),NOW());