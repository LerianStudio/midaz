ALTER TABLE "transaction" DROP CONSTRAINT IF EXISTS fk_transaction_route_id;
ALTER TABLE "transaction" DROP COLUMN IF EXISTS route_id;
