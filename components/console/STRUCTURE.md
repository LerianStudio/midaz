# Console Structure Overview

Welcome to the comprehensive guide on the structure of our console project, which is designed with a focus on scalability, maintainability, and clear separation of concerns following the Clean Architecture principles within a Next.js application. This architecture enhances our project's efficiency and performance while ensuring that our codebase is organized in a way that allows developers to navigate and contribute effectively.

## Directory Layout

The project is structured into several key directories, each serving specific roles:

```
├── locales/                # Internationalization files
│   ├── compiled/           # Compiled translation files
│   └── extracted/          # Extracted translation strings
├── public/                 # Static assets
│   ├── animations/         # Animation files
│   ├── images/             # Image assets
│   └── svg/                # SVG assets
├── scripts/                # Utility scripts
├── services/               # Service configurations
│   └── configs/            # Configuration files for services
└── src/                    # Source code
    ├── app/                # Next.js App Router structure
    │   ├── (auth-routes)/  # Unauthenticated routes
    │   │   ├── signin/     # Sign in page
    │   │   └── signout/    # Sign out page
    │   ├── (routes)/       # Authenticated routes
    │   │   ├── [...not_found]/            # 404 page
    │   │   ├── accounts/                  # Accounts pages
    │   │   ├── assets/                    # Assets pages
    │   │   ├── ledgers/                   # Ledgers pages
    │   │   ├── onboarding/                # Onboarding flow
    │   │   │   ├── create/                # Create onboarding
    │   │   │   ├── ledger/                # Ledger onboarding
    │   │   │   └── onboard-dialog/        # Onboarding dialogs
    │   │   ├── portfolios/                # Portfolios pages
    │   │   ├── segments/                  # Segments pages
    │   │   ├── settings/                  # Settings pages
    │   │   │   ├── organizations/         # Organization settings
    │   │   │   └── users/                 # User management
    │   │   └── transactions/              # Transactions pages
    │   │       ├── [transactionId]/       # Transaction details
    │   │       └── create/                # Create transaction
    │   └── api/                                # API routes
    │       ├── admin/                          # Admin endpoints
    │       │   └── health/                     # Health check endpoints
    │       │       ├── alive/                  # Liveness probe (GET)
    │       │       └── ready/                  # Readiness probe (GET)
    │       ├── auth/                           # Authentication endpoints
    │       │   └── [...nextauth]/              # NextAuth.js integration
    │       ├── identity/                       # Identity management
    │       │   ├── groups/                     # Group management
    │       │   │   ├── [groupId]/              # Specific group operations (GET, PUT, DELETE)
    │       │   │   └── /                       # Group listing/creation (GET, POST)
    │       │   └── users/                      # User management
    │       │       ├── [userId]/               # Specific user operations (GET, PUT, DELETE)
    │       │       │   ├── password/           # Password management (PUT)
    │       │       │   │   └── admin/          # Admin password operations (PATCH)
    │       │       └── /                       # User listing/creation (GET, POST)
    │       ├── onboarding/                     # Onboarding process API
    │       │   ├── [id]/                       # Specific onboarding flow
    │       │   │   └── complete/               # Complete onboarding (POST)
    │       │   └── /                           # Onboarding management (GET, POST)
    │       ├── organizations/                  # Organization management
    │       │   ├── [id]/                       # Specific organization (GET, PUT, DELETE)
    │       │   │   ├── ledgers/                # Organization ledgers
    │       │   │   │   ├── [ledgerId]/         # Specific ledger (GET, PUT, DELETE)
    │       │   │   │   │   ├── accounts/       # Ledger accounts
    │       │   │   │   │   │   ├── [accountId]/  # Specific account operations
    │       │   │   │   │   │   │   └── /          # GET: Retrieve account details
    │       │   │   │   │   │   │               # PATCH: Update account
    │       │   │   │   │   │   │               # DELETE: Remove account
    │       │   │   │   │   │   └── /          # GET: List accounts with pagination
    │       │   │   │   │   │                   # POST: Create a new account
    │       │   │   │   │   ├── accounts-portfolios/ # Account-portfolio relations
    │       │   │   │   │   │   └── /          # GET: List accounts with their portfolios
    │       │   │   │   │   ├── assets/         # Ledger assets
    │       │   │   │   │   │   ├── [assetId]/   # Specific asset operations
    │       │   │   │   │   │   │   └── /          # GET: Retrieve asset details
    │       │   │   │   │   │   │               # PUT: Update asset
    │       │   │   │   │   │   │               # DELETE: Remove asset
    │       │   │   │   │   │   └── /          # GET: List assets
    │       │   │   │   │   │                   # POST: Add asset to ledger
    │       │   │   │   │   ├── portfolios/     # Ledger portfolios
    │       │   │   │   │   │   ├── [portfolioId]/ # Specific portfolio operations
    │       │   │   │   │   │   │   └── /          # GET: Retrieve portfolio details
    │       │   │   │   │   │   │               # PUT: Update portfolio
    │       │   │   │   │   │   │               # DELETE: Remove portfolio
    │       │   │   │   │   │   └── /          # GET: List portfolios
    │       │   │   │   │   │                   # POST: Create a new portfolio
    │       │   │   │   │   ├── portfolios-accounts/ # Portfolio-account relations
    │       │   │   │   │   │   └── /          # GET: List portfolios with their accounts
    │       │   │   │   │   ├── segments/       # Ledger segments
    │       │   │   │   │   │   ├── [segmentId]/ # Specific segment operations
    │       │   │   │   │   │   │   └── /          # GET: Retrieve segment details
    │       │   │   │   │   │   │               # PUT: Update segment
    │       │   │   │   │   │   │               # DELETE: Remove segment
    │       │   │   │   │   │   └── /          # GET: List segments
    │       │   │   │   │   │                   # POST: Create a new segment
    │       │   │   │   │   └── transactions/   # Ledger transactions
    │       │   │   │   │       ├── [transactionId]/ # Specific transaction operations
    │       │   │   │   │       │   └── /          # GET: Retrieve transaction details
    │       │   │   │   │       │               # PATCH: Update transaction
    │       │   │   │   │       ├── json/       # Transaction JSON operations
    │       │   │   │   │       │   └── /          # POST: Create transaction from JSON
    │       │   │   │   │       └── /          # GET: List transactions with pagination
    │       │   │   │   ├── ledgers-assets/     # Ledger-asset relationships
    │       │   │   │   │   └── /          # GET: List assets across multiple ledgers
    │       │   │   │   └── /                   # GET: List ledgers for an organization
    │       │   │   │                       # POST: Create a new ledger
    │       │   ├── parentOrganizations/        # Parent organization relationships (GET)
    │       │   └── /                           # Organization listing/creation (GET, POST)
    │       └── utils/                          # Utility endpoints and helpers
    │           └── api-error-handler.ts        # Centralized API error handling
    ├── client/                # HTTP client implementations
    ├── components/            # UI components
    │   ├── breadcrumb/        # Breadcrumb navigation
    │   ├── card/              # Card components
    │   ├── confirmation-dialog/ # Confirmation dialogs
    │   ├── empty-resource/    # Empty state components
    │   ├── entity-box/        # Entity display boxes
    │   ├── entity-data-table/ # Data tables for entities
    │   ├── form/              # Form components
    │   │   ├── combo-box-field/  # Combo box inputs
    │   │   ├── country-field/    # Country selection
    │   │   ├── input-field/      # Text inputs
    │   │   ├── metadata-field/   # Metadata inputs
    │   │   ├── pagination-limit-field/ # Pagination controls
    │   │   ├── select-field/     # Select dropdowns
    │   │   ├── state-field/      # State selection
    │   │   └── switch-field/     # Toggle switches
    │   ├── header/            # Header components
    │   ├── not-found-content/ # 404 content
    │   ├── organization-switcher/ # Organization selection
    │   ├── page/              # Page layout components
    │   ├── page-footer/       # Page footer components
    │   ├── page-header/       # Page header components
    │   ├── pagination/        # Pagination controls
    │   ├── settings-dropdown/ # Settings menu
    │   ├── sheet/             # Sheet components
    │   ├── sidebar/           # Sidebar navigation
    │   │   └── primitive/     # Sidebar building blocks
    │   ├── table/             # Table components
    │   ├── transactions/      # Transaction-specific components
    │   │   └── primitives/    # Transaction UI primitives
    │   ├── ui/                # Base UI components
    │   │   ├── alert/         # Alert notifications
    │   │   ├── autosize-textarea/ # Resizable text areas
    │   │   ├── avatar/        # User avatars
    │   │   ├── badge/         # Status badges
    │   │   ├── breadcrumb/    # Breadcrumb navigation
    │   │   ├── button/        # Button components
    │   │   ├── card/          # Card containers
    │   │   ├── checkbox/      # Checkbox inputs
    │   │   ├── collapsible/   # Collapsible sections
    │   │   ├── combobox/      # Combo box components
    │   │   ├── command/       # Command interfaces
    │   │   ├── dialog/        # Modal dialogs
    │   │   ├── dropdown-menu/ # Dropdown menus
    │   │   ├── input/         # Input fields
    │   │   ├── input-with-icon/ # Inputs with icons
    │   │   ├── label/         # Form labels
    │   │   ├── loading-button/ # Buttons with loading state
    │   │   ├── paper/         # Paper containers
    │   │   └── popover/       # Popover components
    │   └── user-dropdown/     # User menu dropdown
    ├── core/                  # Core business logic (Clean Architecture)
    │   ├── application/       # Application layer
    │   │   ├── dto/           # Data Transfer Objects
    │   │   ├── mappers/       # Object mappers
    │   │   └── use-cases/     # Business use cases
    │   │       ├── accounts/  # Account-related use cases
    │   │       ├── accounts-with-portfolios/ # Account-portfolio relations
    │   │       ├── assets/    # Asset-related use cases
    │   │       ├── auth/      # Authentication use cases
    │   │       ├── groups/    # Group management use cases
    │   │       ├── ledgers/   # Ledger-related use cases
    │   │       ├── ledgers-assets/ # Ledger-asset relations
    │   │       ├── onboarding/ # Onboarding use cases
    │   │       ├── organizations/ # Organization use cases
    │   │       ├── portfolios/ # Portfolio use cases
    │   │       ├── portfolios-with-accounts/ # Portfolio-account relations
    │   │       ├── segment/   # Segment use cases
    │   │       ├── transactions/ # Transaction use cases
    │   │       └── users/     # User management use cases
    │   ├── domain/            # Domain layer
    │   │   ├── entities/      # Domain entities
    │   │   └── repositories/  # Repository interfaces
    │   │       └── auth/      # Auth repositories
    │   └── infrastructure/    # Infrastructure layer
    │       ├── container-registry/ # Dependency injection
    │       │   ├── logger/    # Logger registration
    │       │   ├── midaz/     # Midaz service registration
    │       │   ├── midaz-plugins/ # Midaz plugin registration
    │       │   ├── observability/ # Observability tools
    │       │   └── use-cases/ # Use case registration
    │       ├── logger/        # Logging implementation
    │       │   └── decorators/ # Logger decorators
    │       ├── midaz/         # Midaz API integration
    │       │   ├── exceptions/ # Midaz-specific exceptions
    │       │   ├── messages/  # Midaz message formats
    │       │   ├── repositories/ # Midaz repository implementations
    │       │   └── services/  # Midaz service implementations
    │       ├── midaz-plugins/ # Midaz plugin integrations
    │       │   ├── auth/      # Authentication plugins
    │       │   └── identity/  # Identity management plugins
    │       ├── next-auth/     # NextAuth integration
    │       ├── observability/ # Observability implementations
    │       └── utils/         # Infrastructure utilities
    │           ├── avatar/    # Avatar utilities
    │           ├── di/        # Dependency injection utilities
    │           ├── files/     # File handling utilities
    │           └── svgs/      # SVG utilities
    ├── exceptions/            # Custom exceptions
    │   └── client/            # Client-side exceptions
    ├── helpers/               # Helper functions
    ├── hooks/                 # Custom React hooks
    ├── lib/                   # Library integrations
    │   ├── fetcher/           # Data fetching utilities
    │   ├── form/              # Form handling
    │   ├── http/              # HTTP utilities
    │   ├── intl/              # Internationalization
    │   ├── languages/         # Language support
    │   ├── lottie/            # Lottie animations
    │   ├── middleware/        # Next.js middleware
    │   ├── search/            # Search functionality
    │   ├── storage/           # Storage utilities
    │   ├── theme/             # Theming
    │   └── zod/               # Zod schema utilities
    ├── providers/             # Provider components
    │   ├── organization-provider/ # Organization context provider
    │   └── permission-provider/ # Permission context provider
    ├── schema/                # Zod schemas
    ├── types/                 # TypeScript type definitions
    └── utils/                 # Utility functions
```

## Key Directories

### Public (`./public`)

- `animations`: Animation files for UI interactions and loading states
- `images`: Static image assets used throughout the application
- `svg`: SVG assets for icons and illustrations

### SRC (`./src`)

- `app`: Next.js App Router structure for both UI pages and API routes
- `client`: HTTP client implementations for communicating with backend services
- `components`: Reusable UI components organized by functionality
- `core`: Business logic following Clean Architecture principles
- `exceptions`: Custom exception handling for better error management
- `helpers`: Helper functions for common tasks
- `hooks`: Custom React hooks for shared functionality
- `lib`: Configuration and implementation of external libraries
- `providers`: Provider components for application-wide functionality
- `schema`: Zod schemas for data validation
- `types`: TypeScript type definitions for type safety
- `utils`: Utility functions for common operations

### App Routes (`./src/app`)

- `(auth-routes)`: Routes accessible to unauthenticated users
  - `signin/`
  - `signout/`
- `(routes)`: Authenticated routes organized by domain
  - `accounts/`
  - `assets/`
  - `ledgers/`
  - `onboarding/`
  - `portfolios/`
  - `segments/`
  - `settings/`
    - `organizations/`
    - `users/`
  - `transactions/`
    - `[transactionId]/`
    - `create/`
- `api`: API routes implementing backend functionality
  - `admin`: Admin endpoints
    - `health`: Health check
      - `alive`: Liveness probe (GET)
      - `ready`: Readiness probe (GET)
  - `auth`: Authentication endpoints
    - `[...nextauth]`: NextAuth.js integration
  - `identity`: Identity management
    - `groups`: Group management
      - `[groupId]`: Specific group operations
    - `users`: User management
      - `[userId]`: Specific user operations
        - `password`: Password management
          - `admin`: Admin password operations
  - `onboarding`: Onboarding process API
    - `[id]`: Specific onboarding flow
      - `complete`: Complete onboarding process
  - `organizations`: Organization management
    - `[id]`: Specific organization operations
      - `ledgers`: Organization ledgers
        - `[ledgerId]`: Specific ledger operations
          - `accounts`: Ledger accounts
            - `[accountId]`: Specific account operations
          - `accounts-portfolios`: Account-portfolio relations
          - `assets`: Ledger assets
            - `[assetId]`: Specific asset operations
          - `portfolios`: Ledger portfolios
            - `[portfolioId]`: Specific portfolio operations
          - `portfolios-accounts`: Portfolio-account relations
          - `segments`: Ledger segments
            - `[segmentId]`: Specific segment operations
          - `transactions`: Ledger transactions
            - `[transactionId]`: Specific transaction operations
          - `json`: Transaction JSON operations
    - `parentOrganizations`: Parent organization relationships
  - `utils`: Utility endpoints

### Components (`./src/components`)

The components directory contains all UI components organized by functionality:

- **Layout Components**: `page`, `page-header`, `page-footer`, `sidebar`, `header`
- **Data Display**: `table`, `entity-data-table`, `entity-box`, `card`
- **Navigation**: `breadcrumb`, `pagination`, `organization-switcher`
- **Form Components**: Various form fields and inputs in the `form` directory
- **UI Primitives**: Base UI components in the `ui` directory that follow design system patterns
- **Domain-Specific**: Components specific to domain entities like `transactions`

### Core Architecture (`./src/core`)

The core follows Clean Architecture principles with three main layers:

- `application`: Contains use cases, DTOs, and mappers that orchestrate business logic
  - `use-cases`: Business logic organized by domain entities
  - `dto`: Data Transfer Objects for passing data between layers
  - `mappers`: Transform data between different representations

- `domain`: Contains business entities and repository interfaces
  - `entities`: Domain models representing business concepts
  - `repositories`: Interfaces defining data access methods

- `infrastructure`: Contains implementations of repositories and external services
  - `container-registry`: Dependency injection container
  - `midaz`: Integration with Midaz API
  - `midaz-plugins`: Plugin extensions for Midaz
  - `next-auth`: Authentication implementation
  - `logger`: Logging implementation
  - `observability`: Monitoring and observability tools

### Providers (`./src/providers`)

Providers implement React Context for state management across the application:

- `organization-provider`: Manages organization context and selection
- `permission-provider`: Handles user permissions and access control

### Library Integrations (`./src/lib`)

The lib directory contains configurations and implementations of external libraries:

- `fetcher`: Data fetching utilities
- `form`: Form handling and validation
- `http`: HTTP client configurations
- `intl`: Internationalization setup
- `middleware`: Next.js middleware for routing and authentication
- `theme`: Theming and styling configurations
- `zod`: Schema validation utilities
