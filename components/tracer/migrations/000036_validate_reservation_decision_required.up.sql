-- Validate the existing rows against the decision CHECK added NOT VALID by the
-- previous migration. VALIDATE CONSTRAINT holds SHARE UPDATE EXCLUSIVE, so
-- reserve admission continues while the table is scanned.
SET LOCAL lock_timeout = '5s';

ALTER TABLE usage_reservations VALIDATE CONSTRAINT usage_reservations_decision_required;
