# Midaz Console Architecture

![banner](../public/images/midaz-banner.png)

## Overview

The Midaz Console is a comprehensive web-based interface for managing the Midaz open-source ledger system. Built using modern React/Next.js technologies and following Clean Architecture principles, the console provides an intuitive way to manage organizations, ledgers, accounts, assets, transactions, and more.

## Technology Stack

### Core Technologies

- **Framework**: Next.js 14.x with App Router
- **Runtime**: Node.js >=18.x LTS
- **Language**: TypeScript 5.x
- **React**: 18.x with React Server Components
- **Styling**: Tailwind CSS 3.x with custom design system
- **State Management**: React Query (TanStack Query) + React Context
- **Authentication**: NextAuth.js 4.x
- **Dependency Injection**: Inversify container with reflect-metadata

### UI/UX Libraries

- **Component Library**: Radix UI primitives
- **Icons**: Lucide React
- **Animations**: Framer Motion + Lottie React
- **Forms**: React Hook Form + Zod validation
- **Data Tables**: TanStack Table
- **Charts**: Chart.js with react-chartjs-2
- **Color Management**: ColorJS

### Development & Testing

- **Testing**: Jest + Testing Library + Playwright E2E
- **Linting**: ESLint + Prettier
- **Storybook**: Component development and documentation
- **Bundling**: Next.js built-in Webpack + SWC

### Observability & Monitoring

- **OpenTelemetry**: Complete observability stack
- **Logging**: Pino with structured logging
- **Monitoring**: Grafana integration ready
- **Health Checks**: Built-in liveness/readiness probes

### Database & Storage

- **Primary Database**: MongoDB (via Mongoose)
- **File Storage**: Configurable (local/cloud)
- **Caching**: Browser storage + React Query cache

## Architecture Principles

### 1. Clean Architecture

The console follows Clean Architecture principles with clear separation of concerns:

```
┌─────────────────────────────────────────────────────────────┐
│                     Presentation Layer                      │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐│
│  │   UI Components │ │   API Routes    │ │     Pages       ││
│  └─────────────────┘ └─────────────────┘ └─────────────────┘│
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐│
│  │   Use Cases     │ │   Mappers       │ │      DTOs       ││
│  └─────────────────┘ └─────────────────┘ └─────────────────┘│
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│                      Domain Layer                           │
│  ┌─────────────────┐ ┌─────────────────┐                   │
│  │    Entities     │ │  Repository     │                   │
│  │                 │ │  Interfaces     │                   │
│  └─────────────────┘ └─────────────────┘                   │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│                  Infrastructure Layer                       │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐│
│  │  Midaz API      │ │  Auth Plugins   │ │   Database      ││
│  │  Integration    │ │                 │ │   MongoDB       ││
│  └─────────────────┘ └─────────────────┘ └─────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

### 2. Dependency Injection

Uses Inversify container for managing dependencies and enabling testability:

```typescript
// Container registration example
container
  .bind<UserRepository>(TYPES.UserRepository)
  .to(MidazUserRepository)
  .inSingletonScope()
```

### 3. Domain-Driven Design

Organizes code around business domains:

- **Organizations** - Multi-tenant organization management
- **Ledgers** - Financial ledger operations
- **Accounts** - Account management within ledgers
- **Assets** - Asset definitions and management
- **Transactions** - Transaction processing and history
- **Portfolios** - Account grouping and management
- **Segments** - Ledger segmentation

## Directory Structure

### `/src/core` - Clean Architecture Implementation

#### Application Layer (`/src/core/application`)

- **`dto/`** - Data Transfer Objects for API communication
- **`mappers/`** - Transform data between layers (Entity ↔ DTO)
- **`use-cases/`** - Business logic orchestration organized by domain

#### Domain Layer (`/src/core/domain`)

- **`entities/`** - Business entities representing core concepts
- **`repositories/`** - Repository interfaces (dependency inversion)

#### Infrastructure Layer (`/src/core/infrastructure`)

- **`container-registry/`** - Dependency injection modules
- **`midaz/`** - Midaz API integration
- **`midaz-plugins/`** - Plugin system (Auth, Identity, etc.)
- **`mongo/`** - MongoDB implementations
- **`next-auth/`** - Authentication provider
- **`observability/`** - Telemetry and monitoring

### `/src/app` - Next.js App Router

#### Route Organization

```
/src/app/
├── (auth-routes)/          # Public authentication pages
│   ├── signin/            # Login page
│   └── signout/           # Logout page
├── (routes)/              # Protected application pages
│   ├── accounts/          # Account management
│   ├── assets/            # Asset management
│   ├── ledgers/           # Ledger management
│   ├── onboarding/        # New user/org onboarding
│   ├── portfolios/        # Portfolio management
│   ├── segments/          # Segment management
│   ├── settings/          # Application settings
│   └── transactions/      # Transaction management
└── api/                   # API endpoints
    ├── admin/             # Admin endpoints
    ├── auth/              # Authentication
    ├── identity/          # User/group management
    ├── onboarding/        # Onboarding API
    ├── organizations/     # Organization API
    └── utils/             # Utility endpoints
```

### `/src/components` - UI Components

#### Component Organization

- **`ui/`** - Base design system components (buttons, inputs, etc.)
- **`form/`** - Form-specific components with validation
- **`table/`** - Data table components
- **`layout/`** - Layout components (header, sidebar, footer)
- **Domain-specific** - Components tied to business entities

### `/src/lib` - Library Integrations

- **`http/`** - HTTP client with error handling
- **`intl/`** - Internationalization (i18n) setup
- **`theme/`** - Theming and styling utilities
- **`form/`** - Form validation and handling
- **`storage/`** - Browser storage utilities

## Data Flow Architecture

### 1. User Interaction Flow

```
User Action → Component → Hook → Use Case → Repository → External API
                ↓
User Feedback ← UI Update ← State Update ← Response ← Mapper ← API Response
```

### 2. API Request Flow

```
Client Request → Next.js API Route → Use Case → Repository → Midaz API
                       ↓
Client Response ← Error Handler ← Mapper ← DTO ← API Response
```

### 3. State Management Flow

```
Component → React Query → Cache → API Client → Midaz API
    ↓           ↓
UI Update ← State Update ← Optimistic Updates
```

## Integration Architecture

### Midaz Core API Integration

The console integrates with multiple Midaz services:

- **Core Ledger API** - Primary ledger operations
- **Onboarding Service** - Organization setup
- **Transaction Service** - Transaction processing
- **Auth Plugin** - Authentication & authorization
- **Identity Plugin** - User/group management

### Plugin System

Extensible plugin architecture for additional functionality:

```typescript
// Plugin structure
interface MidazPlugin {
  name: string
  version: string
  initialize(): Promise<void>
  getRoutes(): Route[]
  getComponents(): Component[]
}
```

## Security Architecture

### Authentication & Authorization

- **NextAuth.js** - Session management
- **JWT Tokens** - API authentication
- **Role-based Access Control** - Permission system
- **Plugin Authentication** - External auth providers

### Security Measures

- **CSRF Protection** - Built-in Next.js CSRF protection
- **XSS Prevention** - Content sanitization
- **Input Validation** - Zod schema validation
- **Secure Headers** - Security headers configuration
- **Environment Isolation** - Environment-specific configurations

## Performance Architecture

### Optimization Strategies

- **Server-Side Rendering** - Next.js SSR/SSG
- **Code Splitting** - Automatic code splitting
- **React Query Cache** - Intelligent data caching
- **Optimistic Updates** - Immediate UI feedback
- **Image Optimization** - Next.js image optimization

### Monitoring & Observability

- **OpenTelemetry** - Distributed tracing
- **Structured Logging** - Pino logger with correlation IDs
- **Performance Metrics** - Web vitals tracking
- **Error Tracking** - Comprehensive error handling

## Development Workflow

### Local Development

```bash
# Environment setup
npm install
npm run set-local-env

# Development server
npm run dev

# Testing
npm run test
npm run test:e2e

# Storybook
npm run storybook
```

### Build & Deployment

```bash
# Production build
npm run build

# Docker deployment
npm run docker-up

# Linting & formatting
npm run lint
npm run format
```

## Testing Strategy

### Testing Layers

1. **Unit Tests** - Jest + Testing Library
2. **Integration Tests** - API route testing
3. **E2E Tests** - Playwright browser testing
4. **Component Tests** - Storybook + component testing
5. **Visual Regression** - Storybook visual testing

### Testing Patterns

- **Test-Driven Development** - Write tests first
- **Page Object Model** - E2E test organization
- **Mock Services** - External API mocking
- **Snapshot Testing** - Component regression testing

## Deployment Architecture

### Production Environment

- **Containerized Deployment** - Docker containers
- **Health Checks** - Liveness/readiness probes
- **Environment Configuration** - Environment-specific settings
- **Monitoring Integration** - Production observability

### Scalability Considerations

- **Horizontal Scaling** - Stateless application design
- **CDN Integration** - Static asset optimization
- **Database Optimization** - MongoDB performance tuning
- **Caching Strategy** - Multi-layer caching

## Future Architecture Considerations

### Planned Enhancements

- **Micro-frontend Architecture** - Plugin-based UI extensions
- **Real-time Updates** - WebSocket integration
- **Advanced Analytics** - Enhanced reporting capabilities
- **Mobile Optimization** - Responsive design improvements
- **API Gateway** - Centralized API management

### Extension Points

- **Plugin System** - Third-party integrations
- **Theme System** - Custom branding support
- **Workflow Engine** - Custom business processes
- **Reporting Engine** - Advanced analytics

---

This architecture documentation provides a comprehensive overview of the Midaz Console's design, implementation patterns, and architectural decisions. It serves as a reference for developers working on the console and helps maintain consistency across the codebase.
