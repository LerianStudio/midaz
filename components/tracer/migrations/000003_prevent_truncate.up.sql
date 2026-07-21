-- ============================================
-- Function: prevent_truncate
-- Trigger function to prevent TRUNCATE on table
-- Required for SOX/GLBA compliance - audit records must never be deleted
-- Uses TG_TABLE_NAME to include table name in error message
-- ============================================

CREATE OR REPLACE FUNCTION prevent_truncate()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'TRUNCATE not allowed on table "%" (SOX/GLBA compliance)', TG_TABLE_NAME;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
