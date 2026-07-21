Feature: Account Manager proactively adjusts limit for a growing customer
  As Paula, a Relationship Manager,
  I want to monitor a customer's limit usage and increase it before transactions are denied,
  So that the customer can continue operating without friction as their business grows.

  Background:
    Given Paula is authenticated in the Tracer system
    And the account for customer "Company ABC" is registered in the system

  # --- Phase 1: Create and activate a daily limit ---

  Scenario: Create and activate a daily limit for Company ABC
    When Paula creates a daily limit called "Company ABC Daily Limit"
    And she sets the maximum amount to R$50,000 in BRL
    And she applies it to the Company ABC account
    Then the limit should be created successfully in Draft status
    When Paula activates the limit "Company ABC Daily Limit"
    Then the limit should become Active

  # --- Phase 2: Submit transactions to approach the limit ---

  Scenario: First transaction is approved
    When a wire transfer of R$25,000 is submitted from Company ABC's account
    Then the transaction should be allowed

  Scenario: Second transaction brings usage near the limit
    When a wire transfer of R$16,000 is submitted from Company ABC's account
    Then the transaction should be allowed

  # --- Phase 3: Check usage shows near-limit state ---

  Scenario: Paula detects that Company ABC is near their limit
    When Paula checks the usage of "Company ABC Daily Limit"
    Then the current usage should be R$41,000
    And the maximum amount should be R$50,000
    And the utilization should be 82%
    And the limit should be flagged as near its threshold

  # --- Phase 4: Increase the limit ---

  Scenario: Paula increases the limit for the growing customer
    When Paula deactivates the limit "Company ABC Daily Limit"
    And she updates the maximum amount to R$80,000
    And she reactivates the limit
    Then the limit should be Active with a maximum of R$80,000

  # --- Phase 5: Transaction approved under new limit and usage reflects headroom ---

  Scenario: Transaction approved under new limit and usage reflects headroom
    When a wire transfer of R$15,000 is submitted from Company ABC's account
    Then the transaction should be allowed
    When Paula checks the usage of "Company ABC Daily Limit"
    Then the current usage should be R$56,000
    And the maximum amount should be R$80,000
    And the limit should not be flagged as near its threshold
