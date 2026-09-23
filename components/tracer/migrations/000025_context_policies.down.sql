-- Removes only the new policy configuration; existing reservations and usage
-- counters are untouched. Published revisions are lost on this rollback.
DROP TABLE evaluation_policy_bindings;
DROP TABLE evaluation_policy_rules;
DROP TABLE evaluation_policy_revisions;
DROP TABLE evaluation_rule_revisions;
DROP FUNCTION check_evaluation_policy_completeness();
DROP FUNCTION reject_evaluation_revision_mutation();
