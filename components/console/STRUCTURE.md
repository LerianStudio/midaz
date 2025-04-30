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
    │   └── api/               # API routes
    │       ├── admin/         # Admin endpoints
    │       │   └── health/    # Health check endpoints
    │       ├── auth/          # Authentication endpoints
    │       │   └── [...nextauth]/ # NextAuth.js routes
    │       ├── identity/      # Identity management
    │       │   ├── groups/    # Group management
    │       │   └── users/     # User management
    │       ├── onboarding/    # Onboarding API
    │       │   └── [id]/      # Specific onboarding
    │       ├── organizations/ # Organization endpoints
    │       │   ├── [id]/      # Specific organization
    │       │   └── parentOrganizations/ # Parent organizations
    │       └── utils/         # Utility endpoints
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

- `(auth-routes)`: Routes accessible to unauthenticated users (signin, signout)
- `(routes)`: Routes requiring authentication, organized by domain entities
  - `accounts`: Account management pages
  - `assets`: Asset management pages
  - `ledgers`: Ledger management pages
  - `onboarding`: User and organization onboarding flow
  - `portfolios`: Portfolio management pages
  - `segments`: Segment management pages
  - `settings`: Application settings pages
  - `transactions`: Transaction management pages
- `api`: API routes implementing the backend functionality
  - `admin`: Administrative endpoints
  - `auth`: Authentication endpoints
  - `identity`: User and group management
  - `onboarding`: Onboarding process API
  - `organizations`: Organization management
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
