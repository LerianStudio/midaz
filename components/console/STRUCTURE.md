# Console Structure Overview

Welcome to the comprehensive guide on the structure of the Midaz Console project. This Next.js application is designed with a focus on scalability, maintainability, and clear separation of concerns following Clean Architecture principles. The architecture enhances efficiency and performance while ensuring the codebase is organized for effective navigation and contribution.

## Technology Stack

| Layer | Technology |
|-------|------------|
| Framework | Next.js 15 (App Router) |
| Language | TypeScript |
| UI Library | React 19 |
| Styling | Tailwind CSS |
| Forms | React Hook Form + Zod |
| State Management | React Context + Zustand |
| Authentication | NextAuth.js |
| Internationalization | next-intl |
| Testing | Jest + Playwright |
| Animations | Lottie |
| Icons | Lucide React |

## Directory Layout

```
console/
├── locales/                # Internationalization files
│   ├── compiled/           # Compiled translation files (JSON)
│   └── extracted/          # Extracted translation strings (PO files)
├── public/                 # Static assets
│   ├── animations/         # Lottie animation files
│   ├── fonts/              # Custom web fonts
│   ├── images/             # Image assets (PNG, JPG, WebP)
│   ├── svg/                # SVG assets and icons
│   └── countries.json      # Country data for forms
├── scripts/                # Utility scripts
│   ├── check-node-version.sh  # Node version validation
│   ├── entrypoint.sh          # Docker entrypoint
│   └── i18n-extract.ts        # Extract i18n strings
├── services/               # Service configurations
│   └── configs/            # External service configs
├── src/                    # Source code
│   ├── app/                # Next.js App Router (pages + API routes)
│   ├── client/             # HTTP client implementations
│   ├── components/         # Reusable UI components
│   ├── core/               # Business logic (Clean Architecture)
│   ├── exceptions/         # Custom exception classes
│   ├── helpers/            # Helper functions
│   ├── hooks/              # Custom React hooks
│   ├── lib/                # Library integrations and configurations
│   ├── providers/          # React Context providers
│   ├── schema/             # Zod validation schemas
│   ├── types/              # TypeScript type definitions
│   └── utils/              # Utility functions
├── tests/                  # Test suites
│   ├── e2e/                # Playwright end-to-end tests
│   ├── fixtures/           # Test fixtures
│   └── utils/              # Test utilities
├── artifacts/              # Build artifacts (gitignored)
├── docker-compose.yml      # Local development Docker setup
├── Dockerfile              # Production container image
├── next.config.mjs         # Next.js configuration
├── tsconfig.json           # TypeScript configuration
├── jest.config.ts          # Jest test configuration
├── playwright.config.ts    # Playwright E2E configuration
├── eslint.config.mjs       # ESLint configuration
├── tailwind.config.js      # Tailwind CSS configuration
└── package.json            # Dependencies and scripts
```

## App Structure (`./src/app`)

### UI Routes

Next.js App Router with route groups for authentication separation:

```
app/
├── (auth-routes)/          # Unauthenticated routes
│   ├── signin/             # Sign-in page
│   └── signout/            # Sign-out confirmation page
├── (routes)/               # Authenticated routes (protected)
│   ├── [...not_found]/     # Custom 404 page
│   ├── account-types/      # Account type management
│   ├── accounts/           # Account management
│   ├── assets/             # Asset management
│   ├── ledgers/            # Ledger management
│   ├── onboarding/         # Organization onboarding flow
│   │   ├── create/         # Create organization step
│   │   ├── ledger/         # Create ledger step
│   │   └── onboard-dialog/ # Onboarding dialog components
│   ├── operation-routes/   # Operation routing rules management
│   ├── portfolios/         # Portfolio management
│   ├── segments/           # Segment management
│   ├── settings/           # Application settings
│   │   ├── organizations/  # Organization settings
│   │   └── users/          # User management
│   ├── transaction-routes/ # Transaction routing rules management
│   ├── transactions/       # Transaction management
│   │   ├── [transactionId]/ # Transaction details page
│   │   └── create/         # Create transaction wizard
│   ├── layout.tsx          # Authenticated layout wrapper
│   └── page.tsx            # Dashboard/home page
├── api/                    # API routes (Next.js API handlers)
├── app.tsx                 # App component wrapper
├── globals.css             # Global styles
└── layout.tsx              # Root layout
```

**Route Group Convention**:
- `(auth-routes)` - Public routes accessible without authentication
- `(routes)` - Protected routes requiring authentication (wrapped in auth middleware)

### API Routes (`./src/app/api`)

Next.js API routes implementing BFF (Backend for Frontend) pattern:

```
api/
├── admin/                  # Administrative endpoints
│   └── health/             # Health check endpoints
│       ├── alive/          # Liveness probe (GET) - returns 200 if app is running
│       └── ready/          # Readiness probe (GET) - checks dependencies
├── auth/                   # Authentication endpoints
│   └── [...nextauth]/      # NextAuth.js catch-all route
│       └── route.ts        # NextAuth configuration and handlers
├── home/                   # Dashboard/home data endpoints
│   └── route.ts            # Dashboard metrics (GET)
├── identity/               # Identity management (proxied to identity service)
│   ├── groups/             # Group management
│   │   ├── [groupId]/      # Specific group operations
│   │   │   └── route.ts    # GET, PUT, DELETE group by ID
│   │   └── route.ts        # GET (list), POST (create) groups
│   └── users/              # User management
│       ├── [userId]/       # Specific user operations
│       │   ├── password/   # Password management
│       │   │   ├── admin/  # Admin password reset
│       │   │   │   └── route.ts  # PATCH (admin reset password)
│       │   │   └── route.ts      # PUT (user change password)
│       │   └── route.ts    # GET, PUT, DELETE user by ID
│       └── route.ts        # GET (list), POST (create) users
├── midaz/                  # Direct Midaz API proxy endpoints
│   └── route.ts            # Generic Midaz API proxy
├── onboarding/             # Onboarding workflow endpoints
│   ├── [id]/               # Specific onboarding flow
│   │   └── complete/       # Complete onboarding
│   │       └── route.ts    # POST (finalize onboarding)
│   └── route.ts            # GET (list), POST (start) onboarding
├── organizations/          # Organization management (proxied to onboarding service)
│   ├── [id]/               # Specific organization operations
│   │   ├── ledgers/        # Organization's ledgers
│   │   │   ├── [ledgerId]/ # Specific ledger operations
│   │   │   │   ├── accounts/ # Ledger accounts
│   │   │   │   │   ├── [accountId]/  # Specific account
│   │   │   │   │   │   └── route.ts  # GET, PATCH, DELETE account
│   │   │   │   │   └── route.ts      # GET (list), POST (create) accounts
│   │   │   │   ├── accounts-portfolios/ # Account-portfolio relations
│   │   │   │   │   └── route.ts      # GET (list with relations)
│   │   │   │   ├── assets/        # Ledger assets
│   │   │   │   │   ├── [assetId]/ # Specific asset
│   │   │   │   │   │   └── route.ts  # GET, PUT, DELETE asset
│   │   │   │   │   └── route.ts      # GET (list), POST (create) assets
│   │   │   │   ├── portfolios/    # Ledger portfolios
│   │   │   │   │   ├── [portfolioId]/ # Specific portfolio
│   │   │   │   │   │   └── route.ts  # GET, PUT, DELETE portfolio
│   │   │   │   │   └── route.ts      # GET (list), POST (create) portfolios
│   │   │   │   ├── portfolios-accounts/ # Portfolio-account relations
│   │   │   │   │   └── route.ts      # GET (list with relations)
│   │   │   │   ├── segments/      # Ledger segments
│   │   │   │   │   ├── [segmentId]/ # Specific segment
│   │   │   │   │   │   └── route.ts  # GET, PUT, DELETE segment
│   │   │   │   │   └── route.ts      # GET (list), POST (create) segments
│   │   │   │   └── transactions/  # Ledger transactions
│   │   │   │       ├── [transactionId]/ # Specific transaction
│   │   │   │       │   └── route.ts  # GET, PATCH transaction
│   │   │   │       ├── json/      # JSON transaction creation
│   │   │   │       │   └── route.ts  # POST (create from JSON)
│   │   │   │       └── route.ts      # GET (list), POST (create) transactions
│   │   │   ├── ledgers-assets/    # Ledger-asset relationships
│   │   │   │   └── route.ts       # GET (list assets across ledgers)
│   │   │   └── route.ts           # GET (list), POST (create) ledgers
│   │   └── route.ts               # GET, PUT, DELETE organization
│   ├── parentOrganizations/       # Parent organization relationships
│   │   └── route.ts               # GET (list parent orgs)
│   └── route.ts                   # GET (list), POST (create) organizations
├── permissions/            # Permission management
│   └── route.ts            # GET (check permissions)
├── plugin/                 # Plugin system endpoints
│   └── route.ts            # GET (list plugins), dynamic plugin routes
└── utils/                  # Utility endpoints
    └── api-error-handler.ts # Centralized error handling middleware
```

**API Route Conventions**:
- All routes follow REST conventions (GET, POST, PUT, PATCH, DELETE)
- Dynamic segments use `[paramName]` syntax
- Most routes proxy to backend services (onboarding, transaction, identity)
- Error handling is centralized in `utils/api-error-handler.ts`
- Authentication is enforced via NextAuth middleware

## Client Layer (`./src/client`)

HTTP client implementations for communicating with backend services:

```
client/
├── account-types.ts        # Account type API client
├── accounts.ts             # Account API client
├── applications.ts         # Application management client
├── assets.ts               # Asset API client
├── balances.ts             # Balance query client
├── groups.ts               # Group management client
├── home.ts                 # Dashboard data client
├── ledgers.ts              # Ledger API client
├── midaz-config.ts         # Midaz configuration client
├── midaz-info.ts           # Midaz info/health client
├── onboarding.ts           # Onboarding flow client
├── operation-routes.ts     # Operation routing client
├── organizations.ts        # Organization API client
├── plugin-menu.ts          # Plugin menu data client
├── portfolios.ts           # Portfolio API client
├── segments.ts             # Segment API client
├── transaction-operation-routes.ts # Transaction operation routing client
├── transaction-routes-cursor.ts    # Transaction routes with cursor pagination
├── transaction-routes.ts   # Transaction routing client
├── transactions.ts         # Transaction API client
└── users.ts                # User management client
```

**Client Responsibilities**:
- Type-safe API calls with TypeScript interfaces
- Request/response transformation
- Error handling and retry logic
- Query parameter construction
- Pagination support (offset and cursor-based)

## Components (`./src/components`)

Reusable UI components organized by functionality:

```
components/
├── breadcrumb/             # Breadcrumb navigation
│   ├── get-breadcrumb-paths.ts # Path generation logic
│   └── index.tsx           # Breadcrumb component
├── card/                   # Card container components
│   ├── card-header.tsx     # Card header
│   ├── card-root.tsx       # Card root container
│   └── index.tsx           # Card exports
├── confirmation-dialog/    # Confirmation modal dialog
│   ├── confirmation-dialog.stories.tsx # Storybook stories
│   ├── confirmation-dialog.mdx         # Documentation
│   ├── index.tsx           # Component implementation
│   ├── use-confirm-dialog.ts   # Hook for dialog state
│   └── use-confirm-dialog.test.ts # Hook tests
├── cursor-pagination/      # Cursor-based pagination controls
│   ├── cursor-pagination.tsx
│   └── index.ts
├── empty-resource/         # Empty state component
│   ├── empty-resource.stories.tsx
│   ├── empty-resource.mdx
│   └── index.tsx
├── entity-box/             # Entity display card
│   ├── entity-box.stories.tsx
│   ├── entity-box.mdx
│   └── index.tsx
├── entity-data-table/      # Data table for entities
│   ├── entity-data-table.stories.tsx
│   ├── entity-data-table.mdx
│   └── index.tsx
├── form/                   # Form field components
│   ├── combo-box-field/    # Combo box input
│   ├── copyable-input-field.tsx # Input with copy button
│   ├── country-field/      # Country selector
│   ├── currency-field/     # Currency input
│   ├── input-field/        # Text input
│   ├── metadata-field/     # Metadata key-value editor
│   ├── pagination-limit-field/ # Pagination size selector
│   ├── password-field.tsx  # Password input with visibility toggle
│   ├── search-account-by-alias-field/ # Account search by alias
│   ├── select-field/       # Dropdown select
│   ├── state-field/        # State/province selector
│   ├── switch-field/       # Toggle switch
│   └── index.tsx           # Form exports
├── header/                 # Application header
├── not-found-content/      # 404 page content
├── organization-switcher/  # Organization selection dropdown
├── page/                   # Page layout wrapper
├── page-footer/            # Page footer components
├── page-header/            # Page header components
├── pagination/             # Offset-based pagination controls
├── settings-dropdown/      # Settings menu dropdown
├── sheet/                  # Slide-out sheet components
├── sidebar/                # Sidebar navigation
│   └── primitive/          # Sidebar building blocks
├── table/                  # Table components
├── transactions/           # Transaction-specific components
│   └── primitives/         # Transaction UI primitives
├── ui/                     # Base UI components (design system)
│   ├── alert/              # Alert notifications
│   ├── autosize-textarea/  # Auto-resizing text area
│   ├── avatar/             # User avatar
│   ├── badge/              # Status badge
│   ├── breadcrumb/         # Breadcrumb primitive
│   ├── button/             # Button variants
│   ├── card/               # Card primitive
│   ├── checkbox/           # Checkbox input
│   ├── collapsible/        # Collapsible section
│   ├── combobox/           # Combo box primitive
│   ├── command/            # Command palette
│   ├── dialog/             # Modal dialog
│   ├── dropdown-menu/      # Dropdown menu
│   ├── input/              # Input primitive
│   ├── input-with-icon/    # Input with icon
│   ├── label/              # Form label
│   ├── loading-button/     # Button with loading state
│   ├── paper/              # Paper container
│   └── popover/            # Popover component
└── user-dropdown/          # User menu dropdown
```

**Component Organization**:
- **Domain Components**: Top-level components specific to business entities
- **ui/**: Base design system components (reusable primitives)
- **form/**: Specialized form field components
- **Storybook**: Components with `.stories.tsx` and `.mdx` files are documented in Storybook

## Core Architecture (`./src/core`)

Clean Architecture implementation with strict layer separation:

```
core/
├── application/            # Application layer (use cases)
│   ├── dto/                # Data Transfer Objects
│   │   ├── accounts-dto.ts
│   │   ├── assets-dto.ts
│   │   ├── auth-dto.ts
│   │   ├── common-dto.ts
│   │   ├── groups-dto.ts
│   │   ├── ledgers-dto.ts
│   │   ├── onboarding-dto.ts
│   │   ├── organizations-dto.ts
│   │   ├── portfolios-dto.ts
│   │   ├── segments-dto.ts
│   │   ├── transactions-dto.ts
│   │   └── users-dto.ts
│   ├── mappers/            # Object mappers (DTO ↔ Entity)
│   │   ├── account-mapper.ts
│   │   ├── asset-mapper.ts
│   │   ├── group-mapper.ts
│   │   ├── ledger-mapper.ts
│   │   ├── organization-mapper.ts
│   │   ├── portfolio-mapper.ts
│   │   ├── segment-mapper.ts
│   │   ├── transaction-mapper.ts
│   │   └── user-mapper.ts
│   └── use-cases/          # Business use cases (one per operation)
│       ├── accounts/       # Account use cases
│       │   ├── create-account.ts
│       │   ├── delete-account.ts
│       │   ├── get-account-by-id.ts
│       │   ├── list-accounts.ts
│       │   └── update-account.ts
│       ├── accounts-with-portfolios/ # Account-portfolio relations
│       ├── assets/         # Asset use cases
│       ├── auth/           # Authentication use cases
│       ├── groups/         # Group management use cases
│       ├── ledgers/        # Ledger use cases
│       ├── ledgers-assets/ # Ledger-asset relations
│       ├── onboarding/     # Onboarding use cases
│       ├── organizations/  # Organization use cases
│       ├── portfolios/     # Portfolio use cases
│       ├── portfolios-with-accounts/ # Portfolio-account relations
│       ├── segment/        # Segment use cases
│       ├── transactions/   # Transaction use cases
│       └── users/          # User management use cases
├── domain/                 # Domain layer (business logic)
│   ├── entities/           # Domain entities (business models)
│   │   ├── account.ts
│   │   ├── asset.ts
│   │   ├── balance.ts
│   │   ├── group.ts
│   │   ├── ledger.ts
│   │   ├── organization.ts
│   │   ├── portfolio.ts
│   │   ├── segment.ts
│   │   ├── transaction.ts
│   │   └── user.ts
│   └── repositories/       # Repository interfaces (ports)
│       └── auth/           # Auth repository interface
└── infrastructure/         # Infrastructure layer (adapters)
    ├── container-registry/ # Dependency injection container
    │   ├── logger/         # Logger registration
    │   ├── midaz/          # Midaz client registration
    │   ├── midaz-plugins/  # Midaz plugin registration
    │   ├── observability/  # Observability tool registration
    │   └── use-cases/      # Use case registration
    ├── logger/             # Logger implementation
    │   └── decorators/     # Logger decorators
    ├── midaz/              # Midaz API integration
    │   ├── exceptions/     # Midaz-specific exceptions
    │   ├── messages/       # Midaz message formats
    │   ├── repositories/   # Midaz repository implementations
    │   └── services/       # Midaz service clients
    ├── midaz-plugins/      # Midaz plugin integrations
    │   ├── auth/           # Authentication plugin
    │   └── identity/       # Identity management plugin
    ├── next-auth/          # NextAuth integration
    ├── observability/      # Observability implementations
    └── utils/              # Infrastructure utilities
        ├── avatar/         # Avatar utilities
        ├── di/             # Dependency injection utilities
        ├── files/          # File handling utilities
        └── svgs/           # SVG utilities
```

**Clean Architecture Layers**:

1. **Domain Layer** (innermost):
   - Business entities (pure TypeScript classes)
   - Repository interfaces (ports)
   - No dependencies on external libraries

2. **Application Layer** (orchestration):
   - Use cases (business logic implementation)
   - DTOs (data contracts for external communication)
   - Mappers (transform between entities and DTOs)
   - Depends only on domain layer

3. **Infrastructure Layer** (outermost):
   - Repository implementations (adapters)
   - External service integrations (Midaz API, Identity service)
   - Dependency injection container
   - Depends on domain and application layers

**Dependency Rule**: Dependencies point inward. Infrastructure → Application → Domain.

## Library Integrations (`./src/lib`)

External library configurations and wrappers:

```
lib/
├── fetcher/                # Data fetching utilities
│   ├── fetcher.ts          # Base fetch wrapper
│   └── use-swr-fetcher.ts  # SWR integration
├── form/                   # Form handling
│   ├── form-context.tsx    # React Hook Form context
│   └── use-form.ts         # Form hook wrapper
├── http/                   # HTTP utilities
│   ├── http-client.ts      # Axios client configuration
│   └── interceptors.ts     # Request/response interceptors
├── intl/                   # Internationalization
│   ├── get-translations.ts # Translation loader
│   └── intl-provider.tsx   # next-intl provider wrapper
├── languages/              # Language support
│   ├── get-languages.ts    # Available languages
│   └── language-detector.ts # Browser language detection
├── lottie/                 # Lottie animations
│   └── lottie-player.tsx   # Lottie player component
├── middleware/             # Next.js middleware
│   ├── auth-middleware.ts  # Authentication middleware
│   └── intl-middleware.ts  # Internationalization middleware
├── search/                 # Search functionality
│   └── use-search.ts       # Search hook with debouncing
├── storage/                # Storage utilities
│   ├── local-storage.ts    # LocalStorage wrapper
│   └── session-storage.ts  # SessionStorage wrapper
├── theme/                  # Theming
│   ├── theme-provider.tsx  # Theme context provider
│   └── use-theme.ts        # Theme hook
└── zod/                    # Zod schema utilities
    └── zod-resolver.ts     # React Hook Form + Zod integration
```

## Providers (`./src/providers`)

React Context providers for global state:

```
providers/
├── organization-provider/  # Organization context
│   ├── organization-context.tsx  # Context definition
│   ├── organization-provider.tsx # Provider component
│   └── use-organization.ts       # Hook for consuming context
└── permission-provider/    # Permission context
    ├── permission-context.tsx    # Context definition
    ├── permission-provider.tsx   # Provider component
    └── use-permission.ts         # Hook for consuming context
```

**Provider Responsibilities**:
- `organization-provider`: Manages selected organization, organization switching, organization data caching
- `permission-provider`: Manages user permissions, role-based access control, permission checking utilities

## Schemas (`./src/schema`)

Zod validation schemas for type-safe form validation:

```
schema/
├── account-schema.ts       # Account validation rules
├── asset-schema.ts         # Asset validation rules
├── auth-schema.ts          # Authentication validation rules
├── ledger-schema.ts        # Ledger validation rules
├── onboarding-schema.ts    # Onboarding validation rules
├── organization-schema.ts  # Organization validation rules
├── portfolio-schema.ts     # Portfolio validation rules
├── segment-schema.ts       # Segment validation rules
├── transaction-schema.ts   # Transaction validation rules
└── user-schema.ts          # User validation rules
```

**Schema Usage**:
- Integrated with React Hook Form via `zod-resolver`
- Runtime type validation + TypeScript type inference
- Error message customization
- Reusable validation rules

## Helpers and Utilities

### Helpers (`./src/helpers`)

Business logic helper functions:

```
helpers/
├── currency-formatter.ts   # Currency formatting
├── date-formatter.ts       # Date/time formatting
├── metadata-parser.ts      # Metadata parsing/serialization
├── permission-checker.ts   # Permission validation logic
├── transaction-calculator.ts # Transaction calculations
└── url-builder.ts          # URL construction
```

### Utilities (`./src/utils`)

General-purpose utility functions:

```
utils/
├── cn.ts                   # Tailwind class name merger (clsx + tailwind-merge)
├── debounce.ts             # Debounce utility
├── format.ts               # General formatting functions
├── random.ts               # Random ID/string generation
├── sort.ts                 # Sorting utilities
└── validation.ts           # Validation helpers
```

## Testing (`./tests`)

### End-to-End Tests (`./tests/e2e`)

Playwright tests for critical user flows:

```
e2e/
├── auth/                   # Authentication flow tests
├── onboarding/             # Onboarding flow tests
├── transactions/           # Transaction creation/management tests
└── settings/               # Settings and user management tests
```

### Test Utilities (`./tests/utils`)

Shared test helpers:

```
utils/
├── test-helpers.ts         # Common test utilities
├── mock-data.ts            # Mock data generators
└── setup-tests.ts          # Test environment setup
```

## Configuration Files

### TypeScript (`tsconfig.json`)

- Strict type checking enabled
- Path aliases configured (`@/` → `src/`)
- Next.js-specific settings

### Next.js (`next.config.mjs`)

- Internationalization (i18n) configuration
- Environment variable handling
- Image optimization settings
- Experimental features (React 19 support)

### Tailwind CSS (`tailwind.config.js`)

- Custom color palette
- Component variants
- Plugin integrations (forms, typography)

### ESLint (`eslint.config.mjs`)

- Next.js recommended rules
- TypeScript-specific rules
- Custom project rules
- Import ordering rules

## Development Workflow

### Scripts (`package.json`)

```bash
# Development
npm run dev              # Start development server (port 3000)
npm run dev:turbo        # Start with Turbopack (faster HMR)

# Building
npm run build            # Production build
npm run start            # Start production server

# Code Quality
npm run lint             # Run ESLint
npm run lint:fix         # Fix ESLint issues
npm run type-check       # TypeScript type checking
npm run format           # Format with Prettier

# Testing
npm run test             # Run Jest unit tests
npm run test:watch       # Jest in watch mode
npm run test:e2e         # Run Playwright E2E tests
npm run test:e2e:ui      # Playwright with UI

# Internationalization
npm run i18n:extract     # Extract translation strings
npm run i18n:compile     # Compile translations

# Storybook
npm run storybook        # Start Storybook dev server
npm run build-storybook  # Build Storybook static site
```

## Environment Variables

Required environment variables (`.env.local`):

```bash
# NextAuth
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=<secret>

# Backend Services
NEXT_PUBLIC_MIDAZ_API_URL=http://localhost:3001
NEXT_PUBLIC_IDENTITY_API_URL=http://localhost:3002

# Feature Flags
NEXT_PUBLIC_ENABLE_PLUGINS=true
NEXT_PUBLIC_ENABLE_ANALYTICS=false

# Observability
NEXT_PUBLIC_SENTRY_DSN=<sentry-dsn>
```

## Key Conventions

### File Naming

- React components: `PascalCase.tsx` (e.g., `AccountList.tsx`)
- Hooks: `use-kebab-case.ts` (e.g., `use-account.ts`)
- Utilities: `kebab-case.ts` (e.g., `format-currency.ts`)
- Types: `kebab-case.ts` with `-types` suffix (e.g., `account-types.ts`)
- Tests: `{name}.test.ts` or `{name}.spec.ts`

### Code Organization

- **One component per file**: Each component in its own file
- **Index exports**: Use `index.ts` for exporting public API
- **Co-location**: Place tests, stories, and docs next to components
- **Barrel exports**: Use index files to group related exports

### React Patterns

- **Functional components**: Always use functional components with hooks
- **Custom hooks**: Extract reusable logic into custom hooks
- **Composition**: Favor composition over inheritance
- **Server components**: Use React Server Components by default (Next.js 15)
- **Client components**: Mark with `'use client'` only when needed

### TypeScript Patterns

- **Strict types**: Enable strict mode, avoid `any`
- **Type inference**: Let TypeScript infer when obvious
- **Interfaces vs Types**: Use `interface` for objects, `type` for unions/intersections
- **Generics**: Use generics for reusable components/functions

## Progressive Disclosure

For detailed documentation on specific topics:

| Topic | Location |
|-------|----------|
| Authentication Flow | `src/core/infrastructure/next-auth/` |
| API Route Patterns | `src/app/api/utils/api-error-handler.ts` |
| Form Validation | `src/schema/` |
| Component Documentation | Storybook (run `npm run storybook`) |
| Clean Architecture | `src/core/` README files |
| Testing Patterns | `tests/` README files |

## Related Resources

- **Root Project**: `../../STRUCTURE.md` - Overall Midaz project structure
- **API Documentation**: `../../components/onboarding/api/swagger.yaml` - Backend API specs
- **Design System**: Run `npm run storybook` for component documentation
- **Contributing**: `../../CONTRIBUTING.md` (if exists)

---

**Last Updated**: 2025-12-14 (Generated from codebase analysis)

**Maintained By**: Lerian Studio

**Technology**: Next.js 15 + React 19 + TypeScript + Clean Architecture
