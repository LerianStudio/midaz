Feature: Risk Manager enforces daily spending limit for corporate segment
  As Ricardo, a Risk Manager at a regional bank,
  I want to create and activate a daily limit for the corporate segment,
  So that I comply with the R$100,000 daily limit policy approved by the board.

  Background:
    Given Ricardo is authenticated in the Tracer system
    And the corporate segment has been registered in the system

  # --- Phase 1: Create and activate the daily limit ---

  Scenario: Create and activate a daily limit for the corporate segment
    When Ricardo creates a daily limit called "Corporate Daily Limit"
    And he sets the maximum amount to R$100,000 in BRL
    And he applies it to payment transactions in the corporate segment
    Then the limit should be created successfully in Draft status
    When Ricardo activates the limit
    Then the limit should become Active

  # --- Phase 2: Transaction within limit is approved ---

  Scenario: Approve a payment within the daily limit
    Given the limit "Corporate Daily Limit" is Active with a maximum of R$100,000
    When a payment of R$30,000 is submitted for the corporate segment
    Then the transaction should be allowed
    And the limit should not be exceeded

  # --- Phase 3: Check usage reflects the transaction ---

  Scenario: Verify limit usage after first transaction
    When Ricardo checks the usage of "Corporate Daily Limit"
    Then the current usage should be R$30,000
    And the maximum amount should be R$100,000
    And the utilization should be 30%
    And the limit should not be flagged as near its threshold

  # --- Phase 4: Build up usage and verify denial ---

  Scenario: Approve another payment approaching the limit
    When a payment of R$60,000 is submitted for the corporate segment
    Then the transaction should be allowed

  Scenario: Deny a payment that would exceed the daily limit
    # Cumulative usage after previous transactions: R$30,000 + R$60,000 = R$90,000
    Given the usage of "Corporate Daily Limit" is R$90,000
    When a payment of R$20,000 is submitted for the corporate segment
    Then the transaction should be denied because the limit was exceeded

  # --- Phase 5: Verify near-limit indicator ---

  Scenario: Verify usage shows near-limit state
    When Ricardo checks the usage of "Corporate Daily Limit"
    Then the current usage should be R$90,000
    And the utilization should be 90%
    And the limit should be flagged as near its threshold

  # --- Phase 6: SubType matching is case-insensitive ---

  Scenario: Limit subType matching is case-insensitive
    # Input "  Buy  " (whitespace + mixed case) must be persisted as "buy"
    # and must be enforced on a validation whose subType is "BUY". A
    # request with a different subType ("sell") must not trigger the limit.
    When Maria creates a daily limit of 1000 BRL called "Crypto Sweep" with sub-type "  Buy  "
    Then the limit should be created successfully in Draft status
    When Maria activates the limit
    Then the limit should become Active
    And the stored limit "Crypto Sweep" should have sub-type "buy"
    Given a card transaction of R$100
    And the sub-type is "BUY"
    When the transaction is submitted
    Then the limit "Crypto Sweep" should have been evaluated
    Given a card transaction of R$100
    And the sub-type is "sell"
    When the transaction is submitted
    Then the limit "Crypto Sweep" should NOT have been evaluated
