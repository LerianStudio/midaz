Feature: Fraud Analyst creates REVIEW rule for international transaction triage
  As Maria, a Fraud Analyst,
  I want to create a REVIEW rule for international transactions from relatively new accounts,
  So that ambiguous transactions are flagged for analysis without blocking legitimate customers.

  Background:
    Given Maria is authenticated in the Tracer system

  # --- Phase 1: Create and activate the REVIEW rule ---

  Scenario: Create and activate a review rule for international transactions from new accounts
    When Maria creates a rule called "Review International New Account"
    And she configures it to flag for review international transactions between R$5,000 and R$15,000 from accounts less than 60 days old
    Then the rule should be created successfully in Draft status
    And the rule action should be Review
    When Maria activates the rule "Review International New Account"
    Then the rule should become Active

  # --- Phase 2: Matching transaction receives REVIEW decision ---

  Scenario: International transaction from new account is flagged for review
    Given a card transaction of R$10,000
    And the account is 30 days old
    And the merchant is "GlobalShop" in the United States
    When the transaction is submitted
    Then the transaction should be flagged for review
    And the review rule should be referenced in the decision
    # The transaction is NOT blocked — it proceeds but is flagged for analysis

  # --- Phase 3: Non-matching transactions are not flagged ---

  Scenario: Domestic transaction from new account is allowed without review
    Given a card transaction of R$10,000
    And the account is 30 days old
    And the merchant is "LocalStore" in Brazil
    When the transaction is submitted
    Then the transaction should be allowed
    And no rules should have matched

  Scenario: International transaction from established account is allowed without review
    Given a card transaction of R$10,000
    And the account is 120 days old
    And the merchant is "GlobalShop" in the United States
    When the transaction is submitted
    Then the transaction should be allowed
    And no rules should have matched

  # --- Phase 4: Analyze flagged transactions ---

  Scenario: Maria queries validation history to find flagged transactions
    When Maria searches for transactions with a review decision
    Then she should find at least one flagged transaction
    And the results should include the international transaction from the new account
