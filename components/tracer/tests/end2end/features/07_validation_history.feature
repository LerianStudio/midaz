Feature: Risk Manager analyzes validation history to measure rule effectiveness
  As Ricardo, a Risk Manager,
  I want to query recent validation data and audit events,
  So that I can identify which rules are blocking the most transactions and optimize them.

  Background:
    Given Ricardo is authenticated in the Tracer system

  # --- Phase 1: Create rules and generate diverse transactions ---

  Scenario: Set up rules and submit diverse transactions for analysis
    Given a deny rule called "Analysis Block CRYPTO" is active, blocking crypto transactions above R$10,000
    And a review rule called "Analysis Flag Wire" is active, flagging wire transfers above R$20,000
    When the following transactions are submitted:
      | type   | amount      | expected outcome |
      | CRYPTO | R$15,000.00 | denied           |
      | CRYPTO | R$5,000.00  | allowed          |
      | WIRE   | R$25,000.00 | flagged          |
      | WIRE   | R$10,000.00 | allowed          |
      | PIX    | R$1,000.00  | allowed          |
    Then each transaction should receive the expected outcome

  # --- Phase 2: Query and filter validation history ---

  Scenario: Query and filter validation history by decision and rule
    When Ricardo queries the validation history
    Then the results should include recent validation records
    And each record should show the decision, amount, transaction type, and processing time
    When Ricardo filters the validation history for denied transactions only
    Then all results should show a deny decision
    And the results should include the crypto transaction of R$15,000
    When Ricardo filters the validation history by the rule "Analysis Block CRYPTO"
    Then the results should show transactions that were matched by that rule

  # --- Phase 3: Analyze audit trail for rule lifecycle ---

  Scenario: Review audit trail for rule operations and denied validations
    When Ricardo queries the audit trail for rule creation events
    Then the results should include events showing when each rule was created
    And each event should have a timestamp and associated context
    When Ricardo queries the audit trail for denied transaction validations
    Then the results should include events with a deny outcome
    And each event should contain the original request and decision details
