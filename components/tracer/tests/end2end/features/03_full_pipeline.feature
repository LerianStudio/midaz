Feature: Complete validation pipeline evaluates rules, checks limits, and records audit
  As the Tracer platform processing automated transactions,
  I want to validate transactions through the complete pipeline of rules and limits,
  So that every transaction receives a correct decision with a full audit trail.

  Background:
    Given the system is authenticated
    And a test account and segment exist

  # --- Phase 1: Set up rules and limits ---

  Scenario: Set up rules and limits for the pipeline
    Given a deny rule called "Pipeline Block High Value"
    And it targets wire transfers above R$50,000
    When the rule is created
    And the rule is activated
    Then the rule should be Active
    Given an allow rule called "Pipeline Allow Known Merchant"
    And it targets transactions from merchant "TrustedCorp"
    When the rule is created
    And the rule is activated
    Then the rule should be Active
    Given a daily limit called "Pipeline Account Limit"
    And the maximum amount is R$40,000 in BRL
    And it applies to the test account on wire transfers
    When the limit is created
    And the limit is activated
    Then the limit should be Active

  # --- Phase 2: Transaction denied by rule — limits are NOT checked ---

  Scenario: High-value transaction is denied by rule and limit usage is unchanged
    When a wire transfer of R$60,000 is submitted from the test account
    Then the transaction should be denied by a rule
    And the rule "Pipeline Block High Value" should be referenced in the decision
    And no limits should have been evaluated
    When the usage of "Pipeline Account Limit" is checked
    Then the current usage should be zero

  # --- Phase 3: Transaction allowed — limit usage updated ---

  Scenario: Normal transaction passes rules, is within the limit, and updates usage
    When a wire transfer of R$25,000 is submitted from the test account
    Then the transaction should be allowed
    And no rules should have matched
    And the limit should not be exceeded
    When the usage of "Pipeline Account Limit" is checked
    Then the current usage should be R$25,000

  # --- Phase 4: Transaction denied by limit ---

  Scenario: Transaction passes rules but exceeds the daily limit
    When a wire transfer of R$20,000 is submitted from the test account
    Then the transaction should be denied because the limit was exceeded
    And no rules should have matched

  # --- Phase 5: Verify audit trail with hash chain ---

  Scenario: Audit trail records all validations with valid hash chain
    When the audit trail for transaction validations is queried
    Then there should be audit events for each validation performed
    And each event should have a unique identifier, timestamp, and integrity hash
    When the integrity of the audit hash chain is verified
    Then the hash chain should be valid
    And the number of events checked should be greater than zero
