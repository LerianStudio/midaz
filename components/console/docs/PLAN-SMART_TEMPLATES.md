# Smart Templates Implementation Plan for Console

## 📋 Project Overview

This document outlines the implementation plan for integrating Smart Templates functionality into the Midaz Console. The goal is to create a comprehensive template management and report generation interface that showcases the powerful capabilities of our Smart Templates plugin - enabling dynamic document creation, template management, and multi-format report generation through an intuitive UI.

## 🎯 Demo Objectives

### Primary Goals

- **Template Management**: Complete template lifecycle management with upload/edit capabilities
- **Visual Template Builder**: Intuitive interface for creating and editing Pongo2 templates
- **Report Generation**: Interactive report creation and download interface
- **Data Source Integration**: Visual representation of database connections and field mappings
- **Multi-format Output**: Support for HTML, PDF, CSV, and JSON report generation
- **Real-time Preview**: Live template preview with sample data

### Success Metrics

- ✅ Upload, edit, and manage template files (.tpl format)
- ✅ Visual template editor with Pongo2 syntax highlighting
- ✅ Interactive report generation wizard
- ✅ Real-time template preview with live data
- ✅ Multi-format report download (HTML, PDF, CSV, JSON)
- ✅ Template analytics and usage tracking
- ✅ Mobile-responsive design

## 🏗️ Architecture Integration

### Console Integration Points

```
/src/app/(routes)/
├── plugins/
│   └── smart-templates/              # Main templates section
│       ├── page.tsx                  # Templates overview dashboard
│       ├── templates/                # Template management
│       │   ├── page.tsx             # Template library/listing
│       │   ├── [id]/                # Template details
│       │   │   ├── page.tsx         # Template view/edit
│       │   │   ├── preview/         # Template preview
│       │   │   └── analytics/       # Template analytics
│       │   ├── create/              # Template upload wizard
│       │   └── editor/              # Template editor
│       ├── reports/                 # Report management
│       │   ├── page.tsx             # Report listing
│       │   ├── [id]/                # Report details
│       │   ├── create/              # Report generation wizard
│       │   └── history/             # Report history
│       ├── data-sources/            # Data source management
│       │   ├── page.tsx             # Data source listing
│       │   ├── [id]/                # Data source details
│       │   └── explorer/            # Schema explorer
│       └── analytics/               # Templates analytics
│           └── page.tsx             # Usage & performance insights
```

### Data Flow Architecture

```
Console UI → Use Cases → Mappers → Repository → Smart Templates API
    ↓           ↓          ↓           ↓              ↓
Components → Business → DTOs → Infrastructure → Manager Service
            Logic                    Layer         Worker Service
```

## 📚 Implementation Phases

### Phase 1: Foundation & Navigation (Priority: HIGH)

**Timeline**: Day 1 (Morning)
**Goal**: Basic structure and navigation setup

#### 1.1 Project Structure Setup

- [ ] Create Smart Templates route structure in `/src/app/(routes)/plugins/smart-templates/`
- [ ] Add "Smart Templates" section to plugins navigation
- [ ] Set up templates-specific layouts and routing
- [ ] Configure breadcrumb navigation
- [ ] Create base page components

#### 1.2 Core Infrastructure

- [ ] Create TypeScript interfaces for template models
- [ ] Set up API client integration for Smart Templates service (using mock data for now)
- [ ] Implement repository pattern for template operations (using mock data for now)
- [ ] Create mock data generators for development
- [ ] Set up error handling and loading states

#### 1.3 Component Library

- [ ] Create template-specific UI components (navigation, dashboard widget)
- [ ] Design template file upload components
- [ ] Build template editor components
- [ ] Create report generation components
- [ ] Implement data source configuration components

### Phase 2: Template Management (Priority: HIGH)

**Timeline**: Day 1 (Afternoon) - Day 2 (Morning)
**Goal**: Complete template CRUD operations

#### 2.1 Template Library Interface

- [ ] Create responsive data table for templates
- [ ] Implement search and filtering by name, category, tags
- [ ] Add status indicators (active/inactive/draft)
- [ ] Include quick actions (edit, duplicate, delete, preview)
- [ ] Add bulk operations support (export, batch delete)

#### 2.2 Template Upload Wizard

- [ ] File upload interface for .tpl files
- [ ] Template metadata form (name, description, category, tags)
- [ ] Field mapping extraction and validation
- [ ] Data source selection and validation
- [ ] Preview step with sample data

#### 2.3 Template Details & Editing

- [ ] Comprehensive template view layout
- [ ] Template file content display with syntax highlighting
- [ ] Field mapping visualization
- [ ] Usage statistics and analytics
- [ ] Version history tracking

### Phase 3: Template Editor (Priority: HIGH)

**Timeline**: Day 2 (Afternoon)
**Goal**: Visual template editing interface

#### 3.1 Code Editor Components

- [ ] Monaco Editor integration with Pongo2 syntax highlighting
- [ ] Template syntax validation and error highlighting
- [ ] Auto-completion for available fields and functions
- [ ] Split view (editor + preview)
- [ ] File management (save, save as, revert)

#### 3.2 Visual Template Builder

- [ ] Drag-and-drop component builder
- [ ] Field insertion helper
- [ ] Custom filter and tag builder
- [ ] Template composition (includes, extends)
- [ ] Visual preview with live data

#### 3.3 Data Source Integration

- [ ] Database connection selector
- [ ] Schema explorer with field browser
- [ ] Query builder interface
- [ ] Field mapping visualization
- [ ] Data validation and testing

### Phase 4: Report Generation (Priority: MEDIUM)

**Timeline**: Day 3 (Morning)
**Goal**: Interactive report creation interface

#### 4.1 Report Generation Wizard

- [ ] Template selection interface
- [ ] Parameter input form
- [ ] Output format selection (HTML, PDF, CSV, JSON)
- [ ] Generation options (locale, timezone, compression)
- [ ] Preview and confirmation step

#### 4.2 Report Management

- [ ] Report listing with status tracking
- [ ] Real-time generation status updates
- [ ] Download interface for completed reports
- [ ] Report history and filtering
- [ ] Batch report generation

#### 4.3 Preview and Testing

- [ ] Live template preview with sample data
- [ ] Parameter testing interface
- [ ] Multi-format preview
- [ ] Performance testing and optimization
- [ ] Error handling and troubleshooting

### Phase 5: Analytics & Monitoring (Priority: MEDIUM)

**Timeline**: Day 3 (Afternoon)
**Goal**: Template insights and performance monitoring

#### 5.1 Dashboard Components

- [ ] Template usage metrics widgets
- [ ] Report generation statistics
- [ ] Performance monitoring
- [ ] Error rate tracking

#### 5.2 Analytics Views

- [ ] Template popularity charts
- [ ] Report generation trends
- [ ] Data source usage analysis
- [ ] Performance optimization insights

### Phase 6: Integration & Polish (Priority: LOW)

**Timeline**: Day 4
**Goal**: Complete integration and demo preparation

#### 6.1 Data Source Management

- [ ] Data source configuration interface
- [ ] Connection testing and validation
- [ ] Schema synchronization
- [ ] Performance monitoring

#### 6.2 Final Polish

- [ ] Responsive design optimization
- [ ] Loading and error states
- [ ] Demo data scenarios
- [ ] Performance optimization
- [ ] Documentation and tooltips

## 🗂️ File Structure Plan

### New Files to Create

```
/src/app/(routes)/plugins/smart-templates/
├── page.tsx                                    # Templates dashboard
├── layout.tsx                                  # Templates section layout
├── templates/
│   ├── page.tsx                               # Template library
│   ├── [id]/
│   │   ├── page.tsx                           # Template details
│   │   ├── preview/
│   │   │   └── page.tsx                       # Template preview
│   │   └── analytics/
│   │       └── page.tsx                       # Template analytics
│   ├── create/
│   │   └── page.tsx                           # Template upload wizard
│   └── editor/
│       └── [id]/
│           └── page.tsx                       # Template editor
├── reports/
│   ├── page.tsx                               # Report listing
│   ├── [id]/
│   │   └── page.tsx                           # Report details
│   ├── create/
│   │   └── page.tsx                           # Report generation wizard
│   └── history/
│       └── page.tsx                           # Report history
├── data-sources/
│   ├── page.tsx                               # Data source listing
│   ├── [id]/
│   │   └── page.tsx                           # Data source details
│   └── explorer/
│       └── page.tsx                           # Schema explorer
└── analytics/
    └── page.tsx                               # Templates analytics dashboard

/src/components/smart-templates/
├── smart-templates-navigation.tsx             # Horizontal navigation
├── smart-templates-dashboard-widget.tsx       # Dashboard integration
├── templates/
│   ├── template-card.tsx                      # Template summary card
│   ├── template-data-table.tsx                # Template listing table
│   ├── template-upload-wizard.tsx             # Upload wizard
│   ├── template-editor.tsx                    # Monaco editor integration
│   ├── template-preview.tsx                   # Live preview component
│   └── template-status-badge.tsx              # Status indicators
├── reports/
│   ├── report-generation-wizard.tsx           # Report creation wizard
│   ├── report-card.tsx                        # Report summary card
│   ├── report-status-tracker.tsx              # Real-time status updates
│   └── report-download-button.tsx             # Download interface
├── data-sources/
│   ├── data-source-selector.tsx               # Connection selector
│   ├── schema-explorer.tsx                    # Database schema browser
│   ├── field-mapping-visualizer.tsx           # Field mapping display
│   └── query-builder.tsx                      # Visual query builder
├── editor/
│   ├── template-code-editor.tsx               # Monaco editor wrapper
│   ├── syntax-highlighter.tsx                 # Pongo2 syntax highlighting
│   ├── field-inserter.tsx                     # Field insertion helper
│   └── preview-panel.tsx                      # Split view preview
└── analytics/
    ├── template-usage-chart.tsx               # Usage visualization
    ├── report-generation-chart.tsx            # Generation statistics
    └── performance-metrics-card.tsx           # Performance displays

/src/core/domain/entities/
├── template.ts                                 # Template entity
├── report.ts                                  # Report entity
├── data-source.ts                             # Data source entity
└── template-field-mapping.ts                  # Field mapping entity

/src/core/application/dto/
├── template-dto.ts                             # Template DTOs
├── report-dto.ts                               # Report DTOs
├── data-source-dto.ts                          # Data source DTOs
└── template-analytics-dto.ts                  # Analytics DTOs

/src/core/application/use-cases/smart-templates/
├── create-template-use-case.ts                 # Create template
├── update-template-use-case.ts                 # Update template
├── generate-report-use-case.ts                 # Generate reports
├── get-template-analytics-use-case.ts          # Analytics data
└── preview-template-use-case.ts                # Template preview

/src/core/infrastructure/smart-templates/
├── smart-templates-repository.ts               # API integration
└── smart-templates-mapper.ts                   # Data mapping

/src/schema/
├── template.ts                                 # Validation schemas
├── report.ts                                   # Report schemas
└── data-source.ts                              # Data source schemas
```

### Files to Modify

```
/src/components/sidebar/index.tsx               # Add Smart Templates navigation
/src/app/(routes)/plugins/page.tsx              # Add Smart Templates dashboard widget
/src/core/infrastructure/container-registry/    # Register template services
```

## 🎨 UI/UX Design Guidelines

### Design System Integration

- **Colors**: Use existing Midaz theme with template-specific accents
- **Typography**: Consistent with current hierarchy
- **Spacing**: Follow established design tokens
- **Icons**: Lucide React icons for consistency
- **Components**: Build on existing UI library

### Smart Templates-Specific UI Patterns

#### Template Status Indicators

- **Active**: Green badge with check icon
- **Inactive**: Gray badge with pause icon
- **Draft**: Yellow badge with pencil icon
- **Processing**: Blue badge with spinner icon

#### Template Type Visualization

- **Document**: File text icon, blue accent
- **Report**: Chart icon, green accent
- **Contract**: Shield icon, purple accent
- **Receipt**: Receipt icon, orange accent

#### Report Status Display

- **Queued**: Clock icon, gray color
- **Processing**: Spinner icon, blue color
- **Completed**: Check icon, green color
- **Failed**: X icon, red color

### Interactive Elements

#### Template Editor Interface

```typescript
interface TemplateEditorProps {
  template: Template
  onChange: (content: string) => void
  onValidate: (content: string) => ValidationResult
  preview?: boolean
  dataSource?: DataSource
}
```

#### Report Generation Design

- **Wizard flow**: Multi-step guided process
- **Real-time updates**: Progress indicators and status
- **Visual feedback**: Icons, animations, status colors
- **Download options**: Multiple format buttons

### Responsive Design

- **Mobile**: Single column with collapsible editor
- **Tablet**: Two column layout (editor + preview)
- **Desktop**: Three column layout with side panels

## 📊 Mock Data Strategy

### Template Examples

```json
{
  "templates": [
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231c",
      "name": "Monthly Account Statement",
      "description": "Detailed monthly statement with transaction history",
      "category": "financial_reports",
      "tags": ["accounting", "statements", "monthly"],
      "status": "active",
      "fileUrl": "templates/monthly-statement.tpl",
      "mappedFields": {
        "midaz_onboarding": {
          "account": ["id", "alias", "status"]
        },
        "midaz_transaction": {
          "balance": ["available", "scale", "account_id"],
          "transaction": ["id", "amount", "description", "created_at"]
        }
      },
      "usageCount": 156,
      "lastUsed": "2025-01-01T00:00:00Z",
      "createdAt": "2024-12-01T00:00:00Z",
      "updatedAt": "2024-12-15T00:00:00Z"
    },
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231d",
      "name": "Transaction Receipt",
      "description": "Standard transaction confirmation receipt",
      "category": "receipts",
      "tags": ["transaction", "receipt", "confirmation"],
      "status": "active",
      "fileUrl": "templates/transaction-receipt.tpl",
      "mappedFields": {
        "midaz_transaction": {
          "transaction": [
            "id",
            "amount",
            "from_account",
            "to_account",
            "created_at"
          ],
          "operation": ["type", "amount", "currency"]
        }
      },
      "usageCount": 2340,
      "lastUsed": "2025-01-01T12:30:00Z",
      "createdAt": "2024-11-15T00:00:00Z",
      "updatedAt": "2024-12-20T00:00:00Z"
    }
  ]
}
```

### Report Examples

```json
{
  "reports": [
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231e",
      "templateId": "01956b69-9102-75b7-8860-3e75c11d231c",
      "templateName": "Monthly Account Statement",
      "status": "completed",
      "format": "pdf",
      "parameters": {
        "account_id": "01956b69-9102-75b7-8860-3e75c11d231f",
        "month": "2024-12",
        "include_metadata": true
      },
      "fileUrl": "reports/statement-2024-12.pdf",
      "fileSize": 245760,
      "generatedAt": "2025-01-01T10:15:00Z",
      "downloadCount": 3,
      "expiresAt": "2025-02-01T00:00:00Z",
      "processingTime": "2.5s"
    },
    {
      "id": "01956b69-9102-75b7-8860-3e75c11d231f",
      "templateId": "01956b69-9102-75b7-8860-3e75c11d231d",
      "templateName": "Transaction Receipt",
      "status": "processing",
      "format": "html",
      "parameters": {
        "transaction_id": "01956b69-9102-75b7-8860-3e75c11d2320"
      },
      "queuePosition": 2,
      "estimatedCompletion": "2025-01-01T12:45:00Z",
      "startedAt": "2025-01-01T12:30:00Z"
    }
  ]
}
```

### Data Source Examples

```json
{
  "dataSources": [
    {
      "id": "midaz_onboarding",
      "name": "Midaz Onboarding Database",
      "type": "postgresql",
      "description": "Core onboarding service database with accounts and ledgers",
      "status": "connected",
      "tables": [
        {
          "name": "account",
          "fields": [
            "id",
            "alias",
            "name",
            "status",
            "ledger_id",
            "created_at"
          ],
          "recordCount": 15420
        },
        {
          "name": "ledger",
          "fields": ["id", "name", "organization_id", "created_at"],
          "recordCount": 48
        }
      ],
      "lastSync": "2025-01-01T12:00:00Z",
      "queryCount": 1250
    },
    {
      "id": "midaz_transaction",
      "name": "Midaz Transaction Database",
      "type": "postgresql",
      "description": "Transaction service database with balances and operations",
      "status": "connected",
      "tables": [
        {
          "name": "balance",
          "fields": [
            "id",
            "account_id",
            "available",
            "scale",
            "currency",
            "updated_at"
          ],
          "recordCount": 15420
        },
        {
          "name": "transaction",
          "fields": ["id", "amount", "description", "status", "created_at"],
          "recordCount": 89340
        }
      ],
      "lastSync": "2025-01-01T12:00:00Z",
      "queryCount": 3200
    }
  ]
}
```

## 🔧 Technical Implementation Details

### State Management

- **React Query**: Server state and caching
- **React Context**: Template editor context
- **Local Storage**: Template drafts and preferences
- **Session Storage**: Wizard progress

### File Handling

- **File Upload**: Drag-and-drop template file upload
- **Monaco Editor**: Code editor for template editing
- **Syntax Highlighting**: Custom Pongo2 syntax highlighting
- **Live Preview**: Real-time template preview

### Report Generation

- **Status Polling**: Real-time report generation status
- **Download Management**: Secure file download with progress
- **Format Selection**: Multi-format output options
- **Progress Tracking**: Visual progress indicators

### Performance Optimization

- **Virtual Scrolling**: Large template lists
- **Code Splitting**: Route-based splitting
- **Lazy Loading**: Template content loading
- **Debouncing**: Real-time preview updates

## 🧪 Testing Strategy

### Component Testing

```typescript
// Example test for TemplateEditor
test('should validate Pongo2 syntax correctly', () => {
  const template = `Hello {{ user.name }}!`
  const result = validatePongo2Syntax(template)
  expect(result.isValid).toBe(true)
})

test('should highlight syntax errors', () => {
  const template = `Hello {{ user.name }`
  const result = validatePongo2Syntax(template)
  expect(result.isValid).toBe(false)
  expect(result.errors).toContain('Unclosed variable tag')
})
```

### Integration Testing

- **Template Upload**: End-to-end template upload flow
- **Report Generation**: Complete report generation process
- **Editor Functionality**: Template editing and preview
- **Data Source Integration**: Field mapping and validation

### E2E Testing (Playwright)

```typescript
test.describe('Smart Templates Management', () => {
  test('should upload and edit template', async ({ page }) => {
    // Navigate to templates section
    // Upload template file
    // Edit template content
    // Verify template functionality
  })

  test('should generate report successfully', async ({ page }) => {
    // Open report generation wizard
    // Select template and parameters
    // Monitor generation progress
    // Download completed report
  })
})
```

## 📈 Demo Scenarios

### Scenario 1: Financial Report Generation

**Setup**: Bank account statement generation
**Flow**:

1. Upload monthly statement template
2. Configure account data source mappings
3. Generate statement for specific account and date range
4. Preview HTML version
5. Download PDF version
6. View generation analytics

### Scenario 2: Transaction Receipt System

**Setup**: Automated receipt generation
**Flow**:

1. Create transaction receipt template
2. Set up real-time data integration
3. Generate receipt for recent transaction
4. Test multiple output formats
5. Demonstrate batch processing
6. Show usage statistics

### Scenario 3: Contract Document Creation

**Setup**: Dynamic contract generation
**Flow**:

1. Upload contract template with variables
2. Configure customer data integration
3. Generate personalized contract
4. Preview with different data sets
5. Export multiple formats
6. Track template performance

## 🚀 Deployment & Demo Preparation

### Demo Environment Setup

- [ ] Populate with realistic template examples
- [ ] Create various report generation scenarios
- [ ] Set up demo data sources with sample data
- [ ] Generate historical analytics data
- [ ] Test all user flows and error cases

### Demo Script

1. **Introduction** (2 min)

   - Overview of document generation challenges
   - Smart Templates solution benefits

2. **Template Management** (5 min)

   - Upload new template
   - Configure field mappings
   - Demonstrate template editor

3. **Report Generation** (5 min)

   - Create report from template
   - Monitor real-time progress
   - Download multiple formats

4. **Data Integration** (3 min)

   - Show data source connections
   - Demonstrate field mapping
   - Test with live data

5. **Analytics Dashboard** (3 min)

   - Template usage insights
   - Report generation metrics
   - Performance optimization

6. **Advanced Features** (2 min)
   - Template versioning
   - Batch report generation
   - Custom filters and tags

### Success Criteria

- [ ] All template operations work smoothly
- [ ] Report generation is fast and reliable
- [ ] Editor provides excellent developer experience
- [ ] Analytics show meaningful insights
- [ ] Performance is responsive across all features
- [ ] Mobile experience is optimized

## 📅 Timeline Summary

### Day 1 (Foundation & Core Features)

- **Morning**: Setup, navigation, and infrastructure
- **Afternoon**: Template listing and management
- **Evening**: Template upload and basic editing

### Day 2 (Editor & Advanced Features)

- **Morning**: Complete template management
- **Afternoon**: Template editor implementation
- **Evening**: Report generation wizard

### Day 3 (Reports & Analytics)

- **Morning**: Report management and tracking
- **Afternoon**: Analytics dashboard
- **Evening**: Data source integration

### Day 4 (Polish & Demo Prep)

- **Morning**: Final UI/UX improvements
- **Afternoon**: Demo scenarios and data
- **Evening**: Testing and rehearsal

### Day 5 (Demo Day)

- **Morning**: Final preparations
- **Afternoon**: Client presentation

## 🎯 Risk Mitigation

### Technical Risks

- **Complex Template Engine**: Start with basic templates, add complexity gradually
- **File Upload Complexity**: Use proven upload libraries and validation
- **Editor Integration**: Leverage Monaco Editor's robust feature set

### Timeline Risks

- **Scope Creep**: Focus on core template management first
- **Complex UI**: Use existing component patterns and libraries
- **Testing Time**: Automate where possible, focus on critical paths

### Demo Risks

- **Template Validation**: Prepare comprehensive template examples
- **Report Generation**: Test thoroughly with various data scenarios
- **Performance**: Optimize critical rendering and processing paths

---

## 🎉 Future Enhancements

### Phase 2 Considerations

- **Real-time Collaboration**: Multi-user template editing
- **Template Marketplace**: Community template sharing
- **Advanced Analytics**: AI-powered template optimization
- **Workflow Integration**: Integration with approval workflows
- **Custom Functions**: User-defined template functions

---

This plan provides a comprehensive roadmap for implementing Smart Templates functionality in the Midaz Console. The phased approach ensures we deliver essential template management and report generation features first while maintaining flexibility for enhancements based on feedback and demo requirements.
