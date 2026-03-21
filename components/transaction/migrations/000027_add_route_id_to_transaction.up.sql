ALTER TABLE "transaction" ADD COLUMN IF NOT EXISTS route_id UUID;
ALTER TABLE "transaction" DROP CONSTRAINT IF EXISTS fk_transaction_route_id;
ALTER TABLE "transaction" ADD CONSTRAINT fk_transaction_route_id FOREIGN KEY (route_id) REFERENCES transaction_route(id) NOT VALID;
