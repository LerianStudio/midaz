Feature: Fraud Analyst contains high-value PIX fraud from new accounts
  As Maria, a Fraud Analyst at a mid-sized fintech,
  I want to create, activate, and adjust a blocking rule for high-value PIX from new accounts,
  So that I can contain fraud losses while avoiding false positives on VIP customers.

  The identified fraud pattern involves PIX transactions above R$15,000
  from accounts less than 7 days old.

  Background:
    Given Maria is authenticated in the Tracer system

  # --- Phase 1: Create and activate the initial blocking rule ---

  Scenario: Create and activate a blocking rule for suspicious PIX transactions
    When Maria creates a rule called "Block High Value PIX New Accounts"
    And she configures it to deny PIX transactions above R$15,000 from accounts less than 7 days old
    Then the rule should be created successfully in Draft status
    And the rule should have a unique identifier
    When Maria activates the rule
    Then the rule should become Active
    And the activation time should be recorded

  # --- Phase 2: Verify the rule blocks a suspicious transaction ---

  Scenario: Suspicious transaction is denied by the rule
    Given the rule "Block High Value PIX New Accounts" is Active
    And a PIX transaction of R$20,000
    And the account is 3 days old
    When the transaction is submitted
    Then the transaction should be denied
    And the denial should reference the blocking rule
    And a reason for the denial should be provided

  # --- Phase 3: Verify the rule does NOT block a legitimate transaction ---

  Scenario: Legitimate transaction from an established account is allowed
    Given the rule "Block High Value PIX New Accounts" is Active
    And a PIX transaction of R$20,000
    And the account is 90 days old
    When the transaction is submitted
    Then the transaction should be allowed
    And no rules should have matched the transaction

  # --- Phase 4: Adjust the rule to exclude VIP customers ---

  Scenario: Deactivate original rule and create adjusted rule excluding VIP accounts
    Given the rule "Block High Value PIX New Accounts" is Active
    When Maria deactivates the rule
    Then the rule should become Inactive
    And the deactivation time should be recorded
    When Maria creates a rule called "Block High PIX New Non-VIP"
    And she configures it to deny PIX above R$15,000 from accounts less than 7 days old, excluding VIP tier customers
    Then the rule should be created successfully in Draft status
    When Maria activates the rule
    Then the rule should become Active
    And the activation time should be recorded

  # --- Phase 5: Verify the adjusted rule behavior ---

  Scenario Outline: Adjusted rule behavior based on customer tier
    Given the rule "Block High PIX New Non-VIP" is Active
    And a PIX transaction of R$20,000
    And the account is 3 days old
    And the customer tier is "<tier>"
    When the transaction is submitted
    Then the transaction should be <decision>

    Examples:
      | tier     | decision |
      | VIP      | allowed  |
      | standard | denied   |

  # --- Phase 6: Verify audit trail ---

  Scenario: Audit trail records the complete rule lifecycle
    When Maria reviews the audit trail for rule operations
    Then she should see events for both rules being created
    And she should see the activation and deactivation events
    And every audit event should have a unique identifier, timestamp, and integrity hash

  # --- Phase 7: SubType matching is case-insensitive ---

  Scenario: Rule subType matching is case-insensitive
    # Input "SELL" must be persisted in canonical (lowercase) form and must
    # match a validation subType regardless of its casing. A request with
    # a different subType (BUY) must not match.
    When Maria creates a rule called "PIX Small" with sub-type "SELL" denying amounts > 100
    Then the rule should be created successfully in Draft status
    When Maria activates the rule
    Then the rule should become Active
    And the stored rule "PIX Small" should have sub-type "sell"
    Given a PIX transaction of R$500
    And the sub-type is "Sell"
    When the transaction is submitted
    Then the rule "PIX Small" should have matched
    Given a PIX transaction of R$500
    And the sub-type is "BUY"
    When the transaction is submitted
    Then the rule "PIX Small" should NOT have matched
