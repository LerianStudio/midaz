# CRM Implementation Plan for Console

## 🚨 CURRENT STATUS (Updated: January 24, 2025)

### 📊 Overall Completion: ~75%

### ✅ What Has Been Implemented:

1. **Core CRM Structure** - All route pages and layouts created
2. **Navigation Integration** - CRM added to sidebar under "Native Plugins" section
3. **Customer Listing Page** - Functional with search, filtering, and card-based display
4. **Customer Detail Pages** - Basic structure implemented
5. **Customer Creation Wizard** - Multi-step form with TypeScript types
6. **Aliases Pages** - Both global and customer-specific alias management
7. **Mock Data Infrastructure** - Complete customer and alias data generators
8. **CRM Dashboard Widget** - Component exists and integrated
9. **UI Components** - Label component and CRM navigation components created
10. **TypeScript Interfaces** - Customer types fully defined

### 🚧 What Needs Immediate Attention:

1. **Missing Components**:
   - ❌ `customer-data-table.tsx` - Not implemented (using inline table in page)
   - ❌ `customer-card.tsx` - Not implemented (using inline card rendering)
2. **Customer Edit Forms** - Edit functionality referenced but no edit pages created
3. **Form Validation** - Schema validation (Zod) not fully implemented
4. **API Integration** - Still using mock data, no real API calls
5. **Loading States** - Basic implementation, needs refinement
6. **Error Handling** - Minimal error handling in current implementation

### ❌ Issues/Blockers Found:

1. **Component Architecture** - The plan mentions separate components (`customer-data-table.tsx`, `customer-card.tsx`) but the implementation uses inline rendering within pages
2. **Edit Routes Missing** - References to `/customers/[id]/edit` but no edit pages created
3. **Performance Optimizations** - No virtual scrolling or memoization implemented yet
4. **Testing** - No test files found for CRM components
5. **Documentation** - Some help links point to non-existent docs

### 🎯 Recommendations for Demo Readiness:

1. **Critical**: Implement edit functionality or remove edit buttons
2. **Important**: Add proper loading and error states
3. **Nice-to-have**: Extract inline components to separate files for maintainability
4. **Optional**: Add basic form validation for customer creation

---

## 📋 Project Overview

This document outlines the implementation plan for integrating CRM (Customer Relationship Management) functionality into the Midaz Console. The goal is to create a comprehensive customer management interface that showcases our CRM plugin capabilities for the Tuesday client demo.

## 🎯 Demo Objectives

### Primary Goals

- **Customer Management**: Complete customer (holder) lifecycle management
- **Account Linking**: Visual representation of customer-to-account relationships
- **Data Security**: Showcase encrypted data handling and display
- **User Experience**: Intuitive interface for complex customer data
- **Real-world Scenarios**: Demonstrate practical business use cases

### Success Metrics

- ✅ Create, view, edit, and delete customers
- ✅ Manage both individual and company profiles
- ✅ Link customers to ledger accounts via aliases
- ✅ Search and filter customer data
- ✅ Display banking and contact information securely
- ✅ Mobile-responsive design

## 🏗️ Architecture Integration

### Console Integration Points

```
/src/app/(routes)/
├── customers/                 # New CRM section
│   ├── page.tsx              # Customer listing
│   ├── [id]/                 # Customer details
│   │   ├── page.tsx          # Customer profile
│   │   ├── edit/             # Edit customer
│   │   └── aliases/          # Customer aliases
│   └── create/               # New customer wizard
└── accounts/                 # Enhanced with CRM links
    └── [id]/
        └── customer/         # Customer association view
```

### Data Flow Architecture

```
Console UI → Use Cases → Mappers → Mock Repository → Mock Data
    ↓           ↓          ↓           ↓              ↓
Components → Business → DTOs → Infrastructure → JSON Files
            Logic                    Layer
```

## 📚 Implementation Phases

### Phase 1: Foundation (Priority: HIGH) ✅ COMPLETED

**Timeline**: Day 1 (Saturday)
**Goal**: Basic structure and navigation

#### 1.1 Project Structure Setup ✅

- [x] Create CRM route structure in `/src/app/(routes)/plugins/crm/`
- [x] Add "Native Plugins" navigation section to sidebar
- [x] Set up CRM-specific horizontal navigation tabs
- [x] Configure proper layout with PageRoot structure
- [x] Implement routing and breadcrumbs

#### 1.2 Mock Data Infrastructure ✅

- [x] Create mock data generators for realistic customer profiles
- [x] Set up customer and alias mock data
- [x] Implement pagination and search functionality
- [x] Add TypeScript interfaces and proper typing

#### 1.3 Development Environment ✅

- [x] Configure Docker development environment with hot-reload
- [x] Set up volume mounting for instant code updates
- [x] Fix all TypeScript language server issues
- [x] Ensure ESLint and build processes pass

### Phase 2: Core Customer Management (Priority: HIGH) ✅ COMPLETED

**Timeline**: Day 1-2 (Saturday-Sunday)
**Goal**: Complete customer management interface

#### 2.1 Customer Listing Interface 🚧 PARTIALLY COMPLETED

- [x] Create responsive customer listing with proper TypeScript types (using cards, not table)
- [x] Implement search by name, document, email functionality
- [x] Add customer type and status indicators with proper styling
- [x] Include basic pagination functionality
- [ ] Add bulk actions (export, delete) functionality - NOT IMPLEMENTED

#### 2.2 Customer Detail Views ✅ COMPLETED

- [x] Design customer profile layout with comprehensive sections
- [x] Display personal/company information with proper data mapping
- [x] Show contact details and addresses with formatting
- [x] Include metadata and audit information
- [x] Add quick action buttons with proper event handlers

#### 2.3 Customer Forms 🚧 PARTIALLY COMPLETED

- [x] Create multi-step customer creation wizard with progress indicator
- [ ] Build customer edit forms with proper validation - NOT IMPLEMENTED
- [x] Implement conditional fields (Natural vs Legal person)
- [x] Add address and contact management
- [ ] Include form validation and error handling - BASIC ONLY

### Phase 3: Account Linking & Aliases (Priority: MEDIUM) ✅ COMPLETED

**Timeline**: Day 2 (Sunday)
**Goal**: Customer-to-account relationship management

#### 3.1 Aliases Management ✅ COMPLETED

- [x] Create alias listing per customer with comprehensive data table
- [x] Implement alias creation forms with banking details
- [x] Add banking details management (account numbers, IBAN, etc.)
- [x] Show ledger/account relationships with proper navigation
- [x] Include alias status and metadata display

#### 3.2 Account Integration ✅ COMPLETED

- [x] Create global aliases management interface
- [x] Add customer context to alias management
- [x] Implement customer-to-account relationship display
- [x] Create relationship visualization with banking details
- [x] Add quick customer actions from aliases pages

### Phase 4: Advanced Features (Priority: LOW) 🚧 PARTIALLY COMPLETED

**Timeline**: Day 2-3 (Sunday-Monday)
**Goal**: Enhanced user experience and demo polish

#### 4.1 Dashboard Integration 🚧 PARTIALLY COMPLETED

- [x] Add CRM metrics to main dashboard with widget
- [x] Create customer analytics widgets showing key statistics
- [ ] Implement recent activity feeds for customer actions - NOT IMPLEMENTED
- [ ] Add quick access shortcuts for CRM functionality - NOT IMPLEMENTED

#### 4.2 Search & Analytics 🚧 PARTIALLY COMPLETED

- [x] Basic customer search functionality with real-time filtering
- [ ] Advanced filtering and sorting by multiple criteria - BASIC ONLY
- [ ] Customer analytics and reports with data visualization - NOT IMPLEMENTED
- [ ] Export functionality for customer data - NOT IMPLEMENTED

#### 4.3 Polish & Demo Preparation 🚧 IN PROGRESS

- [x] Responsive design refinements for all screen sizes
- [ ] Loading states and error handling throughout the application - MINIMAL
- [x] Demo data scenarios with realistic customer profiles
- [ ] Performance optimizations and code quality improvements - NOT IMPLEMENTED

## 🗂️ File Structure Plan

### 🚧 Files Created (PARTIAL IMPLEMENTATION)

```
/src/app/(routes)/plugins/crm/
├── page.tsx                           # ✅ CRM overview page
├── layout.tsx                         # ✅ CRM section layout with horizontal nav
├── customers/
│   ├── page.tsx                       # ✅ Customer listing page (card-based, not table)
│   ├── [id]/
│   │   ├── page.tsx                   # ✅ Customer detail page with profile layout
│   │   ├── edit/                      # ❌ NOT CREATED - Edit functionality missing
│   │   │   └── page.tsx
│   │   └── aliases/
│   │       └── page.tsx               # ✅ Customer aliases management page
│   └── create/
│       └── page.tsx                   # ✅ Customer creation wizard page
└── aliases/
    └── page.tsx                       # ✅ Global aliases management page

/src/components/crm/
├── crm-navigation.tsx                 # ✅ Horizontal navigation component
├── crm-dashboard-widget.tsx           # ✅ Dashboard integration widget
└── customers/
    ├── customer-card.tsx              # ❌ NOT CREATED - Inline rendering used instead
    ├── customer-data-table.tsx        # ❌ NOT CREATED - Card list used instead
    ├── customer-wizard.tsx            # ✅ Multi-step customer creation wizard
    ├── customer-mock-data.ts          # ✅ Mock data generators
    └── customer-types.ts              # ✅ TypeScript interfaces

/src/components/ui/
└── label.tsx                          # ✅ Label component for forms

/src/components/sidebar/
└── index.tsx                          # ✅ Updated with "Native Plugins" section
```

### 📋 Files to Create (REMAINING) - Not Required for Current Demo

All essential CRM functionality has been implemented using a simplified approach with mock data generators. The following files represent a more comprehensive architecture that could be implemented in future iterations:

```
/src/core/domain/entities/               # Domain-driven design entities (future enhancement)
/src/core/application/dto/               # Data transfer objects (future enhancement)
/src/core/application/mappers/           # Entity-DTO mapping (future enhancement)
/src/core/application/use-cases/         # Business logic layer (future enhancement)
/src/core/infrastructure/mock-crm/       # Repository pattern implementation (future enhancement)
/src/schema/                            # Advanced validation schemas (future enhancement)
```

**Note**: Current implementation uses simplified mock data approach which is sufficient for demo purposes and rapid development.

### Files Modified ✅ COMPLETED

```
/src/components/sidebar/index.tsx       # ✅ Added CRM navigation
/src/app/(routes)/page.tsx              # ✅ Added CRM dashboard widgets
```

**Note**: Account enhancement and container registry changes are planned for future integration phases.

## 🎨 UI/UX Design Guidelines

### Design System Integration

- **Colors**: Follow existing Midaz console theme
- **Typography**: Consistent with current font hierarchy
- **Spacing**: Use established spacing tokens
- **Icons**: Lucide React icons for consistency
- **Components**: Build on existing UI component library

### Customer-Specific UI Patterns

#### Customer Cards

```typescript
interface CustomerCardProps {
  customer: Customer
  showActions?: boolean
  compact?: boolean
}
```

#### Customer Status Indicators

- **Active**: Green badge with checkmark
- **Inactive**: Gray badge with pause icon
- **Pending**: Yellow badge with clock icon
- **Blocked**: Red badge with X icon

#### Customer Type Differentiation

- **Natural Person**: User icon, blue accent
- **Legal Person**: Building icon, purple accent

### Responsive Design

- **Mobile**: Single column layout with collapsible sections
- **Tablet**: Two column layout with optimized forms
- **Desktop**: Three column layout with side panels

## 📊 Mock Data Strategy

### Customer Profiles

```json
{
  "naturalPersons": [
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231c",
      "type": "NATURAL_PERSON",
      "name": "Maria Santos Silva",
      "document": "123.456.789-01",
      "externalId": "CUST_2024_001",
      "contact": {
        "primaryEmail": "maria.santos@email.com",
        "mobilePhone": "+55 11 99999-9999"
      },
      "addresses": {
        "primary": {
          "line1": "Rua das Flores, 123",
          "city": "São Paulo",
          "state": "SP",
          "country": "BR",
          "zipCode": "01234-567"
        }
      },
      "naturalPerson": {
        "birthDate": "1985-03-15",
        "gender": "Female",
        "civilStatus": "Married",
        "nationality": "Brazilian"
      },
      "metadata": {
        "customerSince": "2024-01-15",
        "riskLevel": "Low",
        "preferredLanguage": "pt-BR"
      }
    }
  ],
  "legalPersons": [
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231d",
      "type": "LEGAL_PERSON",
      "name": "TechCorp Solutions Ltda",
      "document": "12.345.678/0001-90",
      "externalId": "CORP_2024_001",
      "legalPerson": {
        "tradeName": "TechCorp",
        "activity": "Software Development",
        "type": "Limited Liability Company",
        "foundingDate": "2020-05-10",
        "size": "Medium",
        "representative": {
          "name": "João Silva",
          "document": "987.654.321-00",
          "email": "joao.silva@techcorp.com",
          "role": "CEO"
        }
      }
    }
  ]
}
```

### Banking Scenarios

```json
{
  "aliases": [
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231e",
      "holderId": "01956b69-9102-75b7-8860-3e75c11d231c",
      "ledgerId": "01956b69-9102-75b7-8860-3e75c11d231f",
      "accountId": "01956b69-9102-75b7-8860-3e75c11d2320",
      "bankingDetails": {
        "bankId": "341",
        "branch": "1234",
        "account": "56789-0",
        "type": "CHECKING",
        "iban": "BR1234567890123456789012345",
        "countryCode": "BR"
      }
    }
  ]
}
```

## 🔧 Technical Implementation Details

### State Management

- **React Query**: For server state and caching
- **React Context**: For customer selection state
- **Local Storage**: For user preferences and filters

### Form Handling

- **React Hook Form**: Form state management
- **Zod**: Schema validation
- **Multi-step Forms**: Wizard navigation with validation

### API Integration (Mock)

```typescript
// Mock API endpoints structure
interface CRMApi {
  customers: {
    list: (params: PaginationParams) => Promise<PaginatedResponse<Customer>>
    getById: (id: string) => Promise<Customer>
    create: (data: CreateCustomerInput) => Promise<Customer>
    update: (id: string, data: UpdateCustomerInput) => Promise<Customer>
    delete: (id: string) => Promise<void>
  }
  aliases: {
    listByCustomer: (customerId: string) => Promise<Alias[]>
    create: (customerId: string, data: CreateAliasInput) => Promise<Alias>
    update: (id: string, data: UpdateAliasInput) => Promise<Alias>
    delete: (id: string) => Promise<void>
  }
}
```

### Performance Considerations

- **Virtual Scrolling**: For large customer lists
- **Lazy Loading**: Component code splitting
- **Memoization**: Expensive calculations and renders
- **Debounced Search**: Reduce API calls during typing

## 🧪 Testing Strategy

### Component Testing

- **Unit Tests**: Individual component functionality
- **Integration Tests**: Form submission and validation
- **Visual Tests**: Storybook component documentation

### E2E Testing (Playwright)

```typescript
// Example E2E test scenarios
test.describe('Customer Management', () => {
  test('should create new individual customer', async ({ page }) => {
    // Navigate to customers page
    // Click create customer
    // Fill form steps
    // Verify customer creation
  })

  test('should link customer to account via alias', async ({ page }) => {
    // Navigate to customer detail
    // Create new alias
    // Select ledger and account
    // Verify alias creation
  })
})
```

## 📈 Demo Scenarios

### Scenario 1: Individual Customer Onboarding

**Persona**: Maria Santos Silva (Individual)
**Flow**:

1. Create new individual customer
2. Fill personal information and documents
3. Add contact details and address
4. Link to checking account
5. View complete customer profile

### Scenario 2: Corporate Customer Management

**Persona**: TechCorp Solutions (Company)
**Flow**:

1. Create corporate customer
2. Add company details and representative
3. Create multiple aliases for different accounts
4. Manage banking relationships
5. View account associations

### Scenario 3: Customer-Account Relationship

**Flow**:

1. Browse existing accounts
2. Associate accounts with customers
3. View customer context in transactions
4. Update customer information
5. Track customer activity

## 🚀 Deployment & Demo Preparation

### Demo Environment Setup

- [ ] Populate with realistic demo data
- [ ] Configure demo-specific settings
- [ ] Test all user flows
- [ ] Prepare presentation scenarios

### Demo Script

1. **Dashboard Overview** (2 min)
   - Show CRM metrics and recent activity
2. **Customer Management** (5 min)
   - Create new individual customer
   - Demonstrate search and filtering
3. **Corporate Customers** (3 min)
   - Show company profile
   - Representative management
4. **Account Linking** (3 min)
   - Create alias and banking details
   - Show account-customer relationship
5. **Integration Demo** (2 min)
   - Navigate between accounts and customers
   - Show unified experience

### Success Criteria

- [ ] All demo scenarios work smoothly
- [ ] UI is responsive and polished
- [ ] Data appears realistic and complete
- [ ] Navigation is intuitive
- [ ] Performance is acceptable
- [ ] No critical bugs or errors

## 📅 Timeline Summary

### ✅ Saturday (Day 1) - COMPLETED

- **Morning**: ✅ Foundation setup and routing
- **Afternoon**: ✅ Core customer listing and mock data
- **Evening**: ✅ TypeScript fixes and development environment

### 🚧 Sunday (Day 2) - PARTIALLY COMPLETED

- **Morning**: ✅ Customer detail views and profile pages
- **Afternoon**: 🚧 Customer creation wizard (no edit forms)
- **Evening**: ✅ Aliases and account linking features + Basic Dashboard integration

### 🚧 Monday (Day 3) - IN PROGRESS

- **Completed**: Basic functionality working
- **Missing**: Edit forms, advanced filtering, performance optimizations
- **Partial**: Error handling, loading states, validation

### 🎯 Tuesday (Demo Day)

- **🚧 DEMO READY with limitations - Core features work but polish needed**

## 🎯 Current Status Summary (UPDATED)

### 🚧 **PARTIALLY COMPLETED (~75% Demo Ready)**

#### ✅ What's Working:

- ✅ Complete CRM plugin foundation with proper navigation
- ✅ Customer listing with basic search functionality
- ✅ Customer detail pages with profile layouts
- ✅ Multi-step customer creation wizard
- ✅ Aliases management for customer-account relationships
- ✅ Basic dashboard integration with CRM widget
- ✅ Mock data generators with realistic Brazilian customer data
- ✅ Development environment with hot-reload
- ✅ Responsive design for all screen sizes

#### ❌ What's Missing/Issues:

- ❌ Customer edit functionality (referenced but not implemented)
- ❌ Advanced filtering and sorting
- ❌ Bulk actions (export, delete)
- ❌ Performance optimizations (virtual scrolling, memoization)
- ❌ Comprehensive error handling and loading states
- ❌ Form validation (Zod schemas not implemented)
- ❌ Recent activity feeds
- ❌ Analytics and reporting features
- ❌ Component extraction (inline rendering instead of reusable components)

### 📊 **Phase Completion Status**

1. ✅ **Phase 1**: Foundation and infrastructure (100%)
2. 🚧 **Phase 2**: Core customer management (75%)
3. ✅ **Phase 3**: Account linking and aliases (90%)
4. 🚧 **Phase 4**: Dashboard integration and polish (40%)

### 📊 **Demo Readiness: 75% 🚧**

**CRM implementation is functional for basic demo but needs polish and missing features for production readiness.**

### 🔧 **Quick Fixes for Demo**

1. Remove or disable edit buttons to avoid broken functionality
2. Add basic loading spinners for better UX
3. Implement simple form validation for customer creation
4. Clean up any console errors or warnings

---

This plan provides a comprehensive roadmap for implementing CRM functionality in the Midaz Console. The phased approach ensures we deliver the most critical features first while maintaining code quality and user experience standards.
