# Midaz Entity Relationships

This document provides visual representations of the entity relationships within the Midaz platform.

## Onboarding Component Entity Relationships

```mermaid
classDiagram
    Organization "1" --> "0..1" Organization : has parent
    Organization "1" --> "*" Ledger : contains
    Ledger "1" --> "*" Segment : contains
    Ledger "1" --> "*" Portfolio : contains
    Ledger "1" --> "*" Asset : contains
    Portfolio "1" --> "1" EntityID : associated with
    Portfolio "1" --> "*" Account : contains
    Segment "1" --> "*" Account : categorizes
    Account "1" --> "0..1" Account : has parent
    Account "1" --> "1" Asset : uses
    
    class Organization {
        +ID
        +LegalName
        +DoingBusinessAs
        +LegalDocument
        +Address
        +Status
        +Metadata
    }
    
    class Ledger {
        +ID
        +Name
        +OrganizationID
        +Status
        +Metadata
    }
    
    class Segment {
        +ID
        +Name
        +LedgerID
        +OrganizationID
        +Status
        +Metadata
    }
    
    class Portfolio {
        +ID
        +Name
        +EntityID
        +LedgerID
        +OrganizationID
        +Status
        +Metadata
    }
    
    class Account {
        +ID
        +Name
        +ParentAccountID
        +EntityID
        +AssetCode
        +OrganizationID
        +LedgerID
        +PortfolioID
        +SegmentID
        +Status
        +Alias
        +Type
        +Metadata
    }
    
    class Asset {
        +ID
        +Name
        +Type
        +Code
        +Status
        +LedgerID
        +OrganizationID
        +Metadata
    }
```

## Transaction Component Entity Relationships

```mermaid
classDiagram
    Transaction "1" --> "0..1" Transaction : has parent
    Transaction "1" --> "*" Operation : contains
    Transaction "1" --> "1" Send : defined by
    Send "1" --> "1" Source : contains
    Send "1" --> "1" Distribute : contains
    Source "1" --> "*" FromTo : from accounts
    Distribute "1" --> "*" FromTo : to accounts
    Operation "1" --> "1" Balance : affects
    Operation "1" --> "1" Balance : results in
    FromTo "1" --> "1" Account : references
    Balance "1" --> "1" Account : belongs to
    
    class Transaction {
        +ID
        +ParentTransactionID
        +Description
        +Template
        +Status
        +Amount
        +AmountScale
        +AssetCode
        +ChartOfAccountsGroupName
        +Source
        +Destination
        +LedgerID
        +OrganizationID
        +Body
        +Metadata
    }
    
    class Operation {
        +ID
        +TransactionID
        +Description
        +Type (DEBIT/CREDIT)
        +AssetCode
        +ChartOfAccounts
        +Amount
        +Balance
        +BalanceAfter
        +Status
        +AccountID
        +AccountAlias
        +BalanceID
        +OrganizationID
        +LedgerID
        +Metadata
    }
    
    class Balance {
        +Available
        +OnHold
        +Scale
    }
    
    class FromTo {
        +Account
        +Amount
        +Share
        +Remaining
        +Rate
        +Description
        +ChartOfAccounts
        +Metadata
        +IsFrom
    }
    
    class Send {
        +Asset
        +Value
        +Scale
        +Source
        +Distribute
    }
    
    class Source {
        +Remaining
        +From
    }
    
    class Distribute {
        +Remaining
        +To
    }
```

## Infrastructure Component Services

```mermaid
classDiagram
    InfraComponent --> PostgreSQL : provides
    InfraComponent --> MongoDB : provides
    InfraComponent --> Redis : provides
    InfraComponent --> RabbitMQ : provides
    InfraComponent --> ObservabilityStack : provides
    
    class InfraComponent {
        Infrastructure Services
    }
    
    class PostgreSQL {
        +Primary
        +Replica
        +Structured Data Storage
    }
    
    class MongoDB {
        +Document Database
        +Metadata Storage
    }
    
    class Redis {
        +In-Memory Cache
        +Temporary Storage
    }
    
    class RabbitMQ {
        +Message Broker
        +Queues
        +Exchanges
    }
    
    class ObservabilityStack {
        +Grafana
        +OpenTelemetry
        +Prometheus
        +Tempo
    }
```

## Integration Between Components

```mermaid
flowchart TB
    subgraph "Infrastructure Component"
        PostgreSQL[(PostgreSQL)]
        MongoDB[(MongoDB)]
        Redis[(Redis)]
        RabbitMQ{RabbitMQ}
        Observability[Grafana/OpenTelemetry]
    end
    
    subgraph "Onboarding Component"
        Organization --> Ledger
        Ledger --> Segment
        Ledger --> Portfolio
        Ledger --> Asset
        Portfolio --> Account
        Segment --> Account
        Asset --> Account
    end
    
    subgraph "Transaction Component"
        Transaction --> Operation
        Operation --> Balance
    end
    
    Account --> Balance
    Transaction -.-> Organization
    Transaction -.-> Ledger
    
    Onboarding Component -.-> PostgreSQL
    Onboarding Component -.-> MongoDB
    Onboarding Component -.-> Redis
    Onboarding Component -.-> RabbitMQ
    
    Transaction Component -.-> PostgreSQL
    Transaction Component -.-> MongoDB
    Transaction Component -.-> Redis
    Transaction Component -.-> RabbitMQ
    
    Onboarding Component -.-> Observability
    Transaction Component -.-> Observability
    
    style Organization fill:#f9d5e5,stroke:#333,stroke-width:1px
    style Ledger fill:#f9d5e5,stroke:#333,stroke-width:1px
    style Segment fill:#f9d5e5,stroke:#333,stroke-width:1px
    style Portfolio fill:#f9d5e5,stroke:#333,stroke-width:1px
    style Account fill:#f9d5e5,stroke:#333,stroke-width:1px
    style Asset fill:#f9d5e5,stroke:#333,stroke-width:1px
    
    style Transaction fill:#d5e5f9,stroke:#333,stroke-width:1px
    style Operation fill:#d5e5f9,stroke:#333,stroke-width:1px
    style Balance fill:#d5e5f9,stroke:#333,stroke-width:1px
    
    style PostgreSQL fill:#e5f9d5,stroke:#333,stroke-width:1px
    style MongoDB fill:#e5f9d5,stroke:#333,stroke-width:1px
    style Redis fill:#e5f9d5,stroke:#333,stroke-width:1px
    style RabbitMQ fill:#e5f9d5,stroke:#333,stroke-width:1px
    style Observability fill:#e5f9d5,stroke:#333,stroke-width:1px
``` 