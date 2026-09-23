-- Published snapshots are separate from editable legacy CEL rules. Tenant
-- isolation uses the existing database-per-tenant connection, never a body field.
CREATE TABLE evaluation_rule_revisions (
    rule_id UUID NOT NULL,
    rule_revision BIGINT NOT NULL CHECK (rule_revision > 0),
    expression TEXT NOT NULL CHECK (length(expression) > 0),
    action VARCHAR(6) NOT NULL CHECK (action IN ('ALLOW', 'DENY', 'REVIEW')),
    PRIMARY KEY (rule_id, rule_revision)
);

CREATE TABLE evaluation_policy_revisions (
    policy_id UUID NOT NULL,
    policy_revision BIGINT NOT NULL CHECK (policy_revision > 0),
    default_decision VARCHAR(5) NOT NULL CHECK (default_decision IN ('ALLOW', 'DENY')),
    rule_count INTEGER NOT NULL CHECK (rule_count >= 0),
    published_by TEXT NOT NULL CHECK (length(published_by) > 0),
    published_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (policy_id, policy_revision)
);

CREATE TABLE evaluation_policy_rules (
    policy_id UUID NOT NULL,
    policy_revision BIGINT NOT NULL,
    rule_id UUID NOT NULL,
    rule_revision BIGINT NOT NULL,
    PRIMARY KEY (policy_id, policy_revision, rule_id),
    FOREIGN KEY (policy_id, policy_revision)
        REFERENCES evaluation_policy_revisions(policy_id, policy_revision),
    FOREIGN KEY (rule_id, rule_revision)
        REFERENCES evaluation_rule_revisions(rule_id, rule_revision)
);

CREATE TABLE evaluation_policy_bindings (
    integration_id TEXT NOT NULL CHECK (octet_length(integration_id) BETWEEN 1 AND 256),
    context_id TEXT NOT NULL CHECK (octet_length(context_id) BETWEEN 1 AND 256),
    binding_version BIGINT NOT NULL DEFAULT 1 CHECK (binding_version > 0),
    policy_id UUID NOT NULL,
    policy_revision BIGINT NOT NULL,
    updated_by TEXT NOT NULL CHECK (length(updated_by) > 0),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (integration_id, context_id),
    FOREIGN KEY (policy_id, policy_revision)
        REFERENCES evaluation_policy_revisions(policy_id, policy_revision)
);

CREATE FUNCTION reject_evaluation_revision_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'published evaluation revisions are immutable' USING ERRCODE = '23514';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER immutable_evaluation_rule_revision BEFORE UPDATE OR DELETE ON evaluation_rule_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_evaluation_revision_mutation();
CREATE TRIGGER immutable_evaluation_policy_revision BEFORE UPDATE OR DELETE ON evaluation_policy_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_evaluation_revision_mutation();
CREATE TRIGGER immutable_evaluation_policy_rules BEFORE UPDATE OR DELETE ON evaluation_policy_rules
    FOR EACH ROW EXECUTE FUNCTION reject_evaluation_revision_mutation();

-- At commit, every published snapshot must contain exactly its declared rules.
-- This also prevents appending new members to an already published revision.
CREATE FUNCTION check_evaluation_policy_completeness() RETURNS trigger AS $$
DECLARE
    expected INTEGER;
    actual BIGINT;
BEGIN
    SELECT rule_count INTO expected FROM evaluation_policy_revisions
      WHERE policy_id = NEW.policy_id AND policy_revision = NEW.policy_revision;
    SELECT count(*) INTO actual FROM evaluation_policy_rules
      WHERE policy_id = NEW.policy_id AND policy_revision = NEW.policy_revision;
    IF expected IS NULL OR expected <> actual THEN
        RAISE EXCEPTION 'incomplete evaluation policy revision' USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER complete_evaluation_policy_revision
    AFTER INSERT ON evaluation_policy_revisions DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_evaluation_policy_completeness();
CREATE CONSTRAINT TRIGGER complete_evaluation_policy_rules
    AFTER INSERT ON evaluation_policy_rules DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_evaluation_policy_completeness();
