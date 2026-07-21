Feature: Fraud Analyst validates decision precedence across conflicting rules
  As Maria, a Fraud Analyst,
  I want to understand how Tracer resolves conflicting rule decisions,
  So that I can design rule sets with predictable behavior.

  Background:
    Given Maria is authenticated in the Tracer system

  # --- Phase 1: Create three rules with different actions for the same scope ---

  Scenario: Create and activate three conflicting rules for crypto transactions
    When Maria creates an allow rule called "Precedence ALLOW" for all crypto transactions
    And she creates a review rule called "Precedence REVIEW" for all crypto transactions
    And she creates a deny rule called "Precedence DENY" for all crypto transactions
    And all three rules are activated
    Then all three rules should be Active

  # --- Phase 2: DENY wins when all three rules match ---

  Scenario: DENY takes precedence over REVIEW and ALLOW
    When a crypto transaction of R$5,000 is submitted
    Then the transaction should be denied
    And the deny rule should be referenced in the decision
    And all three rules should have been evaluated

  # --- Phase 3: Deactivate DENY — REVIEW should win ---

  Scenario: Deactivate the DENY rule
    When Maria deactivates the "Precedence DENY" rule
    Then the rule should become Inactive

  Scenario: REVIEW takes precedence when DENY is absent
    When a crypto transaction of R$5,000 is submitted
    Then the transaction should be flagged for review
    And the review rule should be referenced in the decision
    And the deactivated deny rule should not have been evaluated

  # --- Phase 4: Deactivate REVIEW — ALLOW should win ---

  Scenario: Deactivate the REVIEW rule
    When Maria deactivates the "Precedence REVIEW" rule
    Then the rule should become Inactive

  Scenario: ALLOW is the remaining decision
    When a crypto transaction of R$5,000 is submitted
    Then the transaction should be allowed
    And the allow rule should be referenced in the decision

  # --- Phase 5: Verify all decisions are recorded ---

  Scenario: Validation history shows the progression of decisions
    When Maria reviews the validation history for crypto transactions
    Then the records should show a deny decision, then a review decision, then an allow decision
