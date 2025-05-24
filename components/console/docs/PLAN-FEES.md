# Fees Implementation Plan for Console

## 📋 Project Overview

This document outlines the implementation plan for integrating Fees functionality into the Midaz Console. The goal is to create a comprehensive fee management interface that enables organizations to configure, test, and monitor transaction fees through an intuitive UI, showcasing the powerful capabilities of our Fees plugin.

## 🎯 Demo Objectives

### Primary Goals

- **Fee Package Management**: Complete lifecycle management for fee configurations
- **Visual Rule Builder**: Intuitive interface for creating complex fee structures
- **Fee Calculator**: Interactive tool for testing fee calculations
- **Analytics Dashboard**: Real-time insights into fee revenue and usage
- **Integration Demo**: Show seamless integration with transactions

### Success Metrics

- ✅ Create, edit, and manage fee packages
- ✅ Visual representation of fee calculation rules
- ✅ Real-time fee estimation tool
- ✅ Fee analytics and reporting
- ✅ Transaction integration with automatic fee application
- ✅ Mobile-responsive design

## 🏗️ Architecture Integration

### Console Integration Points

```
/src/app/(routes)/
├── fees/                        # Main fees section
│   ├── page.tsx                # Fees overview dashboard
│   ├── packages/               # Package management
│   │   ├── page.tsx           # Package listing
│   │   ├── [id]/              # Package details
│   │   │   ├── page.tsx       # Package view/edit
│   │   │   └── analytics/     # Package analytics
│   │   └── create/            # New package wizard
│   ├── calculator/            # Fee calculation tool
│   │   └── page.tsx          # Interactive calculator
│   └── analytics/             # Fee analytics
│       └── page.tsx          # Revenue & insights
└── transactions/              # Enhanced with fees
    └── [id]/
        └── fees/             # Transaction fee details
```

### Data Flow Architecture

```
Console UI → Use Cases → Mappers → Repository → Fees API
    ↓           ↓          ↓           ↓           ↓
Components → Business → DTOs → Infrastructure → Service
            Logic                    Layer
```

## 📚 Implementation Phases

### Phase 1: Foundation & Navigation (Priority: HIGH)

**Timeline**: Day 1 (Morning)
**Goal**: Basic structure and navigation setup

#### 1.1 Project Structure Setup

- [ ] Create Fees route structure in `/src/app/(routes)/fees/`
- [ ] Add "Fees" section to main navigation sidebar
- [ ] Set up fees-specific layouts and routing
- [ ] Configure breadcrumb navigation
- [ ] Create base page components

#### 1.2 Core Infrastructure

- [ ] Create TypeScript interfaces for fee models
- [ ] Set up API client integration for Fees service
- [ ] Implement repository pattern for fee operations
- [ ] Create mock data generators for development
- [ ] Set up error handling and loading states

#### 1.3 Component Library

- [ ] Create fee-specific UI components
- [ ] Design fee rule visualization components
- [ ] Build calculation type selectors
- [ ] Create account selector components
- [ ] Implement fee preview components

### Phase 2: Fee Package Management (Priority: HIGH)

**Timeline**: Day 1 (Afternoon) - Day 2 (Morning)
**Goal**: Complete package CRUD operations

#### 2.1 Package Listing Interface

- [ ] Create responsive data table for packages
- [ ] Implement search and filtering by name, status
- [ ] Add status indicators (active/inactive)
- [ ] Include quick actions (edit, duplicate, delete)
- [ ] Add bulk operations support

#### 2.2 Package Creation Wizard

- [ ] Multi-step form for package creation
- [ ] Package basic information (name, description)
- [ ] Calculation rules builder
- [ ] Account waiver configuration
- [ ] Preview and validation step

#### 2.3 Package Details & Editing

- [ ] Comprehensive package view layout
- [ ] Rule visualization with priority ordering
- [ ] Inline editing capabilities
- [ ] Version history tracking
- [ ] Activation/deactivation controls

### Phase 3: Visual Rule Builder (Priority: HIGH)

**Timeline**: Day 2 (Afternoon)
**Goal**: Intuitive fee rule configuration

#### 3.1 Calculation Type Components

- [ ] FLAT fee configuration interface
- [ ] PERCENTAGE fee configuration interface
- [ ] MAX_BETWEEN_TYPES selector
- [ ] Visual priority management (drag & drop)
- [ ] Rule validation and conflict detection

#### 3.2 Advanced Rule Configuration

- [ ] Transaction type criteria builder
- [ ] Min/max amount selectors
- [ ] Currency selection
- [ ] Account selector with search
- [ ] Reference amount configuration

#### 3.3 Rule Testing Interface

- [ ] Live preview of rule effects
- [ ] Sample transaction testing
- [ ] Rule conflict visualization
- [ ] Calculation breakdown display

### Phase 4: Fee Calculator Tool (Priority: MEDIUM)

**Timeline**: Day 3 (Morning)
**Goal**: Interactive fee testing interface

#### 4.1 Calculator Interface

- [ ] Transaction input form
- [ ] Package selection dropdown
- [ ] Real-time fee calculation
- [ ] Calculation breakdown view
- [ ] Multiple scenario comparison

#### 4.2 Estimation Features

- [ ] Batch transaction estimation
- [ ] What-if analysis tools
- [ ] Fee impact visualization
- [ ] Export calculation results

### Phase 5: Analytics & Reporting (Priority: MEDIUM)

**Timeline**: Day 3 (Afternoon)
**Goal**: Fee insights and monitoring

#### 5.1 Dashboard Components

- [ ] Fee revenue metrics widgets
- [ ] Package usage statistics
- [ ] Waived fees tracking
- [ ] Transaction volume analysis

#### 5.2 Analytics Views

- [ ] Time-series fee charts
- [ ] Package performance comparison
- [ ] Account-level fee analysis
- [ ] Export and reporting tools

### Phase 6: Integration & Polish (Priority: LOW)

**Timeline**: Day 4
**Goal**: Complete integration and demo preparation

#### 6.1 Transaction Integration

- [ ] Fee details in transaction views
- [ ] Automatic fee calculation display
- [ ] Fee breakdown in transaction history
- [ ] Fee reversal support

#### 6.2 Final Polish

- [ ] Responsive design optimization
- [ ] Loading and error states
- [ ] Demo data scenarios
- [ ] Performance optimization
- [ ] Documentation and tooltips

## 🗂️ File Structure Plan

### New Files to Create

```
/src/app/(routes)/fees/
├── page.tsx                              # Fees dashboard
├── layout.tsx                            # Fees section layout
├── packages/
│   ├── page.tsx                         # Package listing
│   ├── [id]/
│   │   ├── page.tsx                     # Package details
│   │   └── analytics/
│   │       └── page.tsx                 # Package analytics
│   └── create/
│       └── page.tsx                     # Package creation wizard
├── calculator/
│   └── page.tsx                         # Fee calculator
└── analytics/
    └── page.tsx                         # Fee analytics dashboard

/src/components/fees/
├── fee-navigation.tsx                    # Horizontal navigation
├── fee-dashboard-widget.tsx              # Dashboard integration
├── packages/
│   ├── package-card.tsx                  # Package summary card
│   ├── package-data-table.tsx            # Package listing table
│   ├── package-wizard.tsx                # Creation wizard
│   └── package-status-badge.tsx          # Status indicators
├── rules/
│   ├── rule-builder.tsx                  # Visual rule builder
│   ├── rule-card.tsx                     # Rule display card
│   ├── calculation-type-selector.tsx     # Type selector
│   └── rule-priority-manager.tsx         # Priority ordering
├── calculator/
│   ├── fee-calculator-form.tsx           # Calculator interface
│   ├── calculation-result.tsx            # Result display
│   └── calculation-breakdown.tsx         # Detailed breakdown
└── analytics/
    ├── fee-revenue-chart.tsx             # Revenue visualization
    ├── package-usage-chart.tsx           # Usage statistics
    └── fee-metrics-card.tsx              # Metric displays

/src/core/domain/entities/
├── fee-package.ts                        # Package entity
├── fee-rule.ts                           # Rule entity
└── fee-calculation.ts                    # Calculation entity

/src/core/application/dto/
├── fee-package-dto.ts                    # Package DTOs
├── fee-calculation-dto.ts                # Calculation DTOs
└── fee-analytics-dto.ts                  # Analytics DTOs

/src/core/application/use-cases/fees/
├── create-package-use-case.ts            # Create package
├── update-package-use-case.ts            # Update package
├── calculate-fee-use-case.ts             # Calculate fees
├── get-package-analytics-use-case.ts     # Analytics data
└── estimate-fee-use-case.ts              # Fee estimation

/src/core/infrastructure/fees/
├── fees-repository.ts                    # API integration
└── fees-mapper.ts                        # Data mapping

/src/schema/
├── fee-package.ts                        # Validation schemas
└── fee-calculation.ts                    # Calculation schemas
```

### Files to Modify

```
/src/components/sidebar/index.tsx         # Add Fees navigation
/src/app/(routes)/page.tsx               # Add Fees dashboard widget
/src/app/(routes)/transactions/[id]/page.tsx  # Add fee details
/src/core/infrastructure/container-registry/  # Register fee services
```

## 🎨 UI/UX Design Guidelines

### Design System Integration

- **Colors**: Use existing Midaz theme with fee-specific accents
- **Typography**: Consistent with current hierarchy
- **Spacing**: Follow established design tokens
- **Icons**: Lucide React icons for consistency
- **Components**: Build on existing UI library

### Fee-Specific UI Patterns

#### Package Status Indicators

- **Active**: Green badge with check icon
- **Inactive**: Gray badge with pause icon
- **Draft**: Yellow badge with pencil icon
- **Archived**: Red badge with archive icon

#### Calculation Type Visualization

- **FLAT**: Dollar sign icon, blue accent
- **PERCENTAGE**: Percent icon, green accent
- **MAX_BETWEEN**: Compare icon, purple accent

#### Priority Display

- **Visual hierarchy**: Larger numbers = higher priority
- **Drag handles**: For reordering rules
- **Color coding**: Priority levels
- **Conflict indicators**: Warning badges

### Interactive Elements

#### Rule Builder Interface

```typescript
interface RuleBuilderProps {
  rule: FeeRule
  onChange: (rule: FeeRule) => void
  onValidate: (rule: FeeRule) => ValidationResult
  preview?: boolean
}
```

#### Fee Calculator Design

- **Split view**: Input on left, results on right
- **Real-time updates**: As user types
- **Visual breakdown**: Pie/bar charts
- **Scenario comparison**: Side-by-side view

### Responsive Design

- **Mobile**: Single column with collapsible sections
- **Tablet**: Two column layout for rule builder
- **Desktop**: Three column layout with preview panel

## 📊 Mock Data Strategy

### Fee Package Examples

```json
{
  "packages": [
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231c",
      "name": "Standard Transaction Fees",
      "ledgerId": "main-ledger",
      "active": true,
      "types": [
        {
          "priority": 1,
          "type": "PERCENTAGE",
          "transactionType": {
            "minValue": 100,
            "currency": "USD"
          },
          "calculationType": [{
            "percentage": 2.5,
            "refAmount": "ORIGINAL",
            "origin": ["fees-revenue"],
            "target": ["merchant-account"]
          }]
        }
      ],
      "waivedAccounts": ["vip-123", "vip-456"],
      "metadata": {
        "category": "standard",
        "approvedBy": "admin@company.com"
      }
    },
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231d",
      "name": "Premium Merchant Fees",
      "ledgerId": "main-ledger",
      "active": true,
      "types": [
        {
          "priority": 1,
          "type": "FLAT",
          "calculationType": [{
            "value": 0.30,
            "fromTo": ["fees-fixed"],
            "fromToType": "ORIGIN"
          }]
        },
        {
          "priority": 2,
          "type": "PERCENTAGE",
          "calculationType": [{
            "percentage": 1.5,
            "refAmount": "FEES"
          }]
        }
      ]
    }
  ]
}
```

### Calculation Scenarios

```json
{
  "scenarios": [
    {
      "name": "Small Transaction",
      "transaction": {
        "amount": 50.00,
        "from": "customer-123",
        "to": "merchant-456"
      },
      "result": {
        "fees": 1.25,
        "breakdown": [
          { "type": "PERCENTAGE", "amount": 1.25 }
        ]
      }
    },
    {
      "name": "Large Transaction with Multiple Fees",
      "transaction": {
        "amount": 10000.00,
        "from": "customer-789",
        "to": "merchant-012"
      },
      "result": {
        "fees": 250.30,
        "breakdown": [
          { "type": "FLAT", "amount": 0.30 },
          { "type": "PERCENTAGE", "amount": 250.00 }
        ]
      }
    }
  ]
}
```

## 🔧 Technical Implementation Details

### State Management

- **React Query**: Server state and caching
- **React Context**: Selected package context
- **Local Storage**: Calculator history
- **Session Storage**: Wizard progress

### Form Handling

- **React Hook Form**: Complex form management
- **Zod**: Schema validation
- **Field Arrays**: Dynamic rule lists
- **Conditional Fields**: Type-based rendering

### Data Visualization

- **Chart.js**: Fee analytics charts
- **React DnD**: Drag-and-drop for priorities
- **Framer Motion**: Smooth animations
- **Custom SVG**: Rule flow diagrams

### Performance Optimization

- **Virtual Scrolling**: Large package lists
- **Memoization**: Complex calculations
- **Debouncing**: Real-time calculator
- **Code Splitting**: Route-based splitting

## 🧪 Testing Strategy

### Component Testing

```typescript
// Example test for FeeCalculator
test('should calculate percentage fee correctly', () => {
  const result = calculateFee({
    amount: 100,
    package: mockPackage,
    type: 'PERCENTAGE'
  })
  expect(result.totalFees).toBe(2.5)
})
```

### Integration Testing

- **API Integration**: Mock service responses
- **Form Submission**: End-to-end flows
- **Calculator Accuracy**: Various scenarios
- **Rule Validation**: Edge cases

### E2E Testing (Playwright)

```typescript
test.describe('Fee Package Management', () => {
  test('should create new fee package', async ({ page }) => {
    // Navigate to fees section
    // Click create package
    // Fill wizard steps
    // Verify package creation
  })

  test('should calculate fees correctly', async ({ page }) => {
    // Open calculator
    // Enter transaction details
    // Select package
    // Verify calculation results
  })
})
```

## 📈 Demo Scenarios

### Scenario 1: E-commerce Platform Fees

**Setup**: Online marketplace with multiple fee tiers
**Flow**:

1. Create tiered fee structure
2. Configure percentage + flat fees
3. Set up VIP account waivers
4. Test with sample transactions
5. View analytics dashboard

### Scenario 2: Banking Transaction Fees

**Setup**: Traditional banking fee model
**Flow**:

1. Create account type-based fees
2. Configure minimum balance waivers
3. Set up international transaction fees
4. Calculate complex scenarios
5. Generate fee reports

### Scenario 3: Fintech Innovation

**Setup**: Modern fintech with dynamic pricing
**Flow**:

1. Create time-based fee variations
2. Configure volume discounts
3. Set up promotional periods
4. Test edge cases
5. Analyze fee optimization

## 🚀 Deployment & Demo Preparation

### Demo Environment Setup

- [ ] Populate with realistic fee packages
- [ ] Create various calculation scenarios
- [ ] Set up demo accounts with waivers
- [ ] Generate historical analytics data
- [ ] Test all user flows

### Demo Script

1. **Introduction** (2 min)
   - Overview of fee management challenges
   - Midaz Fees solution benefits

2. **Package Management** (5 min)
   - Create new fee package
   - Configure complex rules
   - Demonstrate priority system

3. **Visual Rule Builder** (5 min)
   - Build multi-tier fee structure
   - Show drag-and-drop priority
   - Validate rules in real-time

4. **Fee Calculator** (3 min)
   - Test various scenarios
   - Compare different packages
   - Export results

5. **Analytics Dashboard** (3 min)
   - Fee revenue insights
   - Package performance
   - Optimization opportunities

6. **Integration Demo** (2 min)
   - Transaction with automatic fees
   - Fee details and breakdown
   - Audit trail

### Success Criteria

- [ ] All CRUD operations work smoothly
- [ ] Rule builder is intuitive and visual
- [ ] Calculator provides accurate results
- [ ] Analytics show meaningful insights
- [ ] Performance is responsive
- [ ] Mobile experience is optimized

## 📅 Timeline Summary

### Day 1 (Foundation & Core Features)

- **Morning**: Setup, navigation, and infrastructure
- **Afternoon**: Package listing and basic CRUD
- **Evening**: Package creation wizard

### Day 2 (Rule Builder & Advanced Features)

- **Morning**: Complete package management
- **Afternoon**: Visual rule builder implementation
- **Evening**: Testing and validation

### Day 3 (Calculator & Analytics)

- **Morning**: Fee calculator tool
- **Afternoon**: Analytics dashboard
- **Evening**: Integration with transactions

### Day 4 (Polish & Demo Prep)

- **Morning**: Final UI/UX improvements
- **Afternoon**: Demo scenarios and data
- **Evening**: Testing and rehearsal

### Day 5 (Demo Day)

- **Morning**: Final preparations
- **Afternoon**: Client presentation

## 🎯 Risk Mitigation

### Technical Risks

- **Complex Rule Logic**: Start with simple rules, add complexity gradually
- **Performance Issues**: Implement caching and optimization early
- **Integration Challenges**: Use mock data initially, integrate incrementally

### Timeline Risks

- **Scope Creep**: Focus on core features first
- **Complex UI**: Use existing component patterns
- **Testing Time**: Automate where possible

### Demo Risks

- **Data Quality**: Prepare comprehensive mock data
- **Edge Cases**: Test thoroughly before demo
- **Performance**: Optimize critical paths

---

This plan provides a comprehensive roadmap for implementing Fees functionality in the Midaz Console. The phased approach ensures we deliver essential features first while maintaining flexibility for enhancements based on feedback and demo requirements.