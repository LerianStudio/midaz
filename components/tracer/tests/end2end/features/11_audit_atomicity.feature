Feature: Audit atomicity during lifecycle operations
  As an operator I need rule and limit lifecycle mutations to roll back if
  their audit event fails to persist so compliance (SOX/GLBA) remains intact.

  Background:
    Given Maria is authenticated in the Tracer system

  # --- Rule activation rolls back when the audit insert fails ---

  Scenario: Rule activation rolls back when audit insert fails
    When Maria creates a rule called "Fraud Block" that denies amounts > 1000
    Then the rule should be created successfully in Draft status
    When the audit event insert is forced to fail for that rule on activation
    And Maria activates the rule
    Then the activation request returns an HTTP 5xx error
    And the rule remains in Draft status
    And no audit event is recorded for that rule activation

  # --- Limit activation rolls back when the audit insert fails ---

  Scenario: Limit activation rolls back when audit insert fails
    When Maria creates a daily spending limit of 500 USD called "Daily Cap"
    Then the limit should be created successfully in Draft status
    When the audit event insert is forced to fail for that limit on activation
    And Maria activates the limit
    Then the activation request returns an HTTP 5xx error
    And the limit remains unchanged
    And no audit event is recorded for that limit activation
