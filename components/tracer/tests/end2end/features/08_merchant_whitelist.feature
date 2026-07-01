Feature: Fraud Analyst creates ALLOW whitelist for trusted merchants
  As Maria, a Fraud Analyst,
  I want to create a rule that explicitly whitelists trusted merchants,
  So that their transactions are clearly marked as approved by policy in the audit trail.

  Background:
    Given Maria is authenticated in the Tracer system
    And merchants "SuperMart" and "FuelCo" have been identified as trusted with zero fraud history

  # --- Phase 1: Create and activate the whitelist rule ---

  Scenario: Create and activate the trusted merchant whitelist rule
    When Maria creates a rule called "Trusted Merchant Whitelist"
    And she configures it to explicitly allow transactions below R$50,000 from "SuperMart" and "FuelCo"
    Then the rule should be created successfully in Draft status
    And the rule action should be Allow
    When Maria activates the rule "Trusted Merchant Whitelist"
    Then the rule should become Active

  # --- Phase 2: Trusted merchant transactions are explicitly approved ---

  Scenario Outline: Whitelisted merchant transaction is matched by the whitelist
    When a card transaction of R$<amount> from merchant "<merchant>" is submitted
    Then the transaction should be allowed
    And the whitelist rule should be referenced in the decision

    Examples:
      | merchant  | amount |
      | SuperMart | 300    |
      | FuelCo    | 500    |

  # --- Phase 3: Non-trusted merchants are NOT matched ---

  Scenario: Transaction from an unknown merchant is allowed by default, not by whitelist
    When a card transaction of R$300 from merchant "UnknownShop" is submitted
    Then the transaction should be allowed
    But no rules should have matched
    # This is a default allow — no explicit whitelist approval

  # --- Phase 4: Audit trail distinguishes explicit vs default approvals ---

  Scenario: Audit trail distinguishes whitelisted approvals from default approvals
    When Maria reviews the audit trail for allowed transaction validations
    Then the SuperMart transaction should show the whitelist rule in its audit context
    And the UnknownShop transaction should show no matched rules in its audit context
    # This distinction lets Maria track which approvals came from policy vs from absence of rules

  # --- Phase 5: Merchant scope enforcement on limits ---

  Scenario: Limit with merchant scope enforces only on matching merchant
    # A limit scoped to a specific merchantId must appear in limitUsageDetails
    # when the validation request carries that merchantId, and must be absent
    # when the merchantId differs.
    When Maria creates a daily limit of 1000 BRL called "Acme Daily Cap" for merchant "acme-corp"
    Then the limit should be created successfully in Draft status
    When Maria activates the limit
    Then the limit should become Active
    Given a card transaction of R$100
    And the merchant has ID "acme-corp"
    When the transaction is submitted
    Then the limit "Acme Daily Cap" should have been evaluated
    Given a card transaction of R$100
    And the merchant has ID "other-merchant"
    When the transaction is submitted
    Then the limit "Acme Daily Cap" should NOT have been evaluated
