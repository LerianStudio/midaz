import {
  Template,
  TemplateStatus,
  TemplateCategory
} from '@/core/domain/entities/template'
import {
  Report,
  ReportStatus,
  ReportFormat
} from '@/core/domain/entities/report'
import {
  DataSource,
  DataSourceStatus,
  DataSourceType
} from '@/core/domain/entities/data-source'

// Mock Templates Data
export const mockTemplates: Template[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231c',
    name: 'Monthly Account Statement',
    description: 'Detailed monthly statement with transaction history',
    category: 'FINANCIAL' as TemplateCategory,
    tags: ['accounting', 'statements', 'monthly'],
    status: 'active' as TemplateStatus,
    fileUrl: 'templates/monthly-statement.tpl',
    mappedFields: {
      midaz_onboarding: {
        table: 'account',
        fields: ['id', 'alias', 'status'],
        queries: ['SELECT * FROM account WHERE id = ?']
      },
      midaz_transaction: {
        table: 'balance',
        fields: ['available', 'scale', 'account_id'],
        queries: ['SELECT * FROM balance WHERE account_id = ?']
      }
    },
    validated: true,
    active: true,
    usageCount: 156,
    lastUsed: '2025-01-01T00:00:00Z',
    metadata: {
      version: '1.0',
      author: 'admin@company.com',
      approval_status: 'approved'
    },
    createdAt: '2024-12-01T00:00:00Z',
    updatedAt: '2024-12-15T00:00:00Z',
    createdBy: 'admin@company.com',
    format: 'PDF',
    engine: 'pongo2',
    version: '1.0',
    dataSourceIds: ['midaz_onboarding', 'midaz_transaction'],
    content: ''
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231d',
    name: 'Transaction Receipt',
    description: 'Standard transaction confirmation receipt',
    category: 'OPERATIONAL' as TemplateCategory,
    tags: ['transaction', 'receipt', 'confirmation'],
    status: 'active' as TemplateStatus,
    fileUrl: 'templates/transaction-receipt.tpl',
    mappedFields: {
      midaz_transaction: {
        table: 'transaction',
        fields: ['id', 'amount', 'from_account', 'to_account', 'created_at'],
        queries: ['SELECT * FROM transaction WHERE id = ?']
      }
    },
    validated: true,
    active: true,
    usageCount: 2340,
    lastUsed: '2025-01-01T12:30:00Z',
    metadata: {
      version: '2.1',
      author: 'template_admin@company.com'
    },
    createdAt: '2024-11-15T00:00:00Z',
    updatedAt: '2024-12-20T00:00:00Z',
    createdBy: 'template_admin@company.com',
    format: 'HTML',
    engine: 'pongo2',
    version: '2.1',
    dataSourceIds: ['midaz_transaction'],
    content: ''
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231e',
    name: 'KYC Verification Report',
    description: 'Customer KYC verification status and documentation',
    category: 'COMPLIANCE' as TemplateCategory,
    tags: ['kyc', 'compliance', 'verification'],
    status: 'draft' as TemplateStatus,
    fileUrl: 'templates/kyc-verification.tpl',
    mappedFields: {
      plugin_crm: {
        table: 'holder',
        fields: ['id', 'name', 'document', 'status'],
        queries: ['SELECT * FROM holder WHERE id = ?']
      }
    },
    validated: false,
    active: false,
    usageCount: 67,
    lastUsed: '2024-12-30T16:45:00Z',
    metadata: {
      version: '0.8',
      author: 'compliance@company.com',
      approval_status: 'pending'
    },
    createdAt: '2024-12-10T00:00:00Z',
    updatedAt: '2024-12-28T00:00:00Z',
    createdBy: 'compliance@company.com',
    format: 'PDF',
    engine: 'pongo2',
    version: '0.8',
    dataSourceIds: ['plugin_crm'],
    content: ''
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231f',
    name: 'Fee Calculation Summary',
    description: 'Summary of fees calculated for transactions',
    category: 'FINANCIAL' as TemplateCategory,
    tags: ['fees', 'billing', 'summary'],
    status: 'active' as TemplateStatus,
    fileUrl: 'templates/fee-summary.tpl',
    mappedFields: {
      plugin_fees: {
        table: 'fee_calculation',
        fields: ['transaction_id', 'fee_amount', 'fee_type'],
        queries: ['SELECT * FROM fee_calculation WHERE transaction_id = ?']
      }
    },
    validated: true,
    active: true,
    usageCount: 423,
    lastUsed: '2025-01-01T08:15:00Z',
    metadata: {
      version: '1.2',
      author: 'finance@company.com'
    },
    createdAt: '2024-11-20T00:00:00Z',
    updatedAt: '2024-12-18T00:00:00Z',
    createdBy: 'finance@company.com',
    format: 'EXCEL',
    engine: 'pongo2',
    version: '1.2',
    dataSourceIds: ['plugin_fees'],
    content: ''
  }
]

// Mock Reports Data
export const mockReports: Report[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2320',
    templateId: '01956b69-9102-75b7-8860-3e75c11d231c',
    templateName: 'Monthly Account Statement',
    status: 'completed',
    format: 'pdf',
    parameters: {
      account_id: '01956b69-9102-75b7-8860-3e75c11d231f',
      month: '2024-12',
      include_metadata: true
    },
    fileUrl: 'reports/statement-2024-12.pdf',
    fileSize: 245760,
    generatedAt: '2025-01-01T10:15:00Z',
    downloadCount: 3,
    expiresAt: '2025-02-01T00:00:00Z',
    processingTime: '2.5s',
    metadata: {
      file_size: 245760,
      processing_time: '2.5s',
      data_sources: ['midaz_onboarding', 'midaz_transaction']
    },
    createdAt: '2025-01-01T10:12:30Z',
    createdBy: 'analyst@company.com'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2321',
    templateId: '01956b69-9102-75b7-8860-3e75c11d231d',
    templateName: 'Transaction Receipt',
    status: 'processing',
    format: 'html',
    parameters: {
      transaction_id: '01956b69-9102-75b7-8860-3e75c11d2322'
    },
    queuePosition: 2,
    estimatedCompletion: '2025-01-01T12:45:00Z',
    startedAt: '2025-01-01T12:30:00Z',
    downloadCount: 0,
    metadata: {
      queue_position: 2,
      estimated_completion: '2025-01-01T12:45:00Z'
    },
    createdAt: '2025-01-01T12:30:00Z',
    createdBy: 'api_user@company.com'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2323',
    templateId: '01956b69-9102-75b7-8860-3e75c11d231e',
    templateName: 'KYC Verification Report',
    status: 'failed',
    format: 'csv',
    parameters: {
      holder_id: '01956b69-9102-75b7-8860-3e75c11d2324'
    },
    startedAt: '2025-01-01T09:00:00Z',
    downloadCount: 0,
    metadata: {
      error: {
        code: 'DATA_SOURCE_ERROR',
        message: 'Failed to connect to CRM database'
      }
    },
    createdAt: '2025-01-01T09:00:00Z',
    createdBy: 'compliance@company.com'
  }
]

// Mock Data Sources
export const mockDataSources: DataSource[] = [
  {
    id: 'midaz_onboarding',
    name: 'Midaz Onboarding Database',
    type: 'postgresql',
    description: 'Core onboarding service database with accounts and ledgers',
    status: 'connected',
    tables: [
      {
        name: 'account',
        fields: ['id', 'alias', 'name', 'status', 'ledger_id', 'created_at'],
        recordCount: 15420,
        lastUpdated: '2025-01-01T12:00:00Z'
      },
      {
        name: 'ledger',
        fields: ['id', 'name', 'organization_id', 'created_at'],
        recordCount: 48,
        lastUpdated: '2025-01-01T12:00:00Z'
      },
      {
        name: 'organization',
        fields: ['id', 'name', 'status', 'created_at'],
        recordCount: 12,
        lastUpdated: '2025-01-01T12:00:00Z'
      }
    ],
    lastSync: '2025-01-01T12:00:00Z',
    queryCount: 1250,
    metadata: {
      host: 'midaz-postgres-primary',
      port: 5432,
      database: 'onboarding'
    },
    createdAt: '2024-10-15T00:00:00Z',
    updatedAt: '2025-01-01T12:00:00Z'
  },
  {
    id: 'midaz_transaction',
    name: 'Midaz Transaction Database',
    type: 'postgresql',
    description: 'Transaction service database with balances and operations',
    status: 'connected',
    tables: [
      {
        name: 'balance',
        fields: [
          'id',
          'account_id',
          'available',
          'scale',
          'currency',
          'updated_at'
        ],
        recordCount: 15420,
        lastUpdated: '2025-01-01T12:00:00Z'
      },
      {
        name: 'transaction',
        fields: ['id', 'amount', 'description', 'status', 'created_at'],
        recordCount: 89340,
        lastUpdated: '2025-01-01T12:00:00Z'
      },
      {
        name: 'operation',
        fields: ['id', 'transaction_id', 'type', 'amount', 'account_id'],
        recordCount: 178680,
        lastUpdated: '2025-01-01T12:00:00Z'
      }
    ],
    lastSync: '2025-01-01T12:00:00Z',
    queryCount: 3200,
    metadata: {
      host: 'midaz-postgres-primary',
      port: 5432,
      database: 'transaction'
    },
    createdAt: '2024-10-20T00:00:00Z',
    updatedAt: '2025-01-01T12:00:00Z'
  },
  {
    id: 'plugin_crm',
    name: 'CRM Plugin Database',
    type: 'mongodb',
    description: 'Customer relationship management data',
    status: 'connected',
    tables: [
      {
        name: 'holder',
        fields: ['id', 'name', 'document', 'type', 'status', 'created_at'],
        recordCount: 8934,
        lastUpdated: '2025-01-01T12:00:00Z'
      },
      {
        name: 'alias',
        fields: ['id', 'holder_id', 'account_id', 'document', 'created_at'],
        recordCount: 12456,
        lastUpdated: '2025-01-01T12:00:00Z'
      }
    ],
    lastSync: '2025-01-01T12:00:00Z',
    queryCount: 567,
    metadata: {
      host: 'midaz-mongodb',
      port: 27017,
      database: 'crm'
    },
    createdAt: '2024-11-01T00:00:00Z',
    updatedAt: '2025-01-01T12:00:00Z'
  }
]

// Template categories
export const templateCategories = [
  'financial_reports',
  'receipts',
  'compliance',
  'contracts',
  'notifications',
  'analytics',
  'statements',
  'invoices'
]

// Mock template content examples
export const mockTemplateContent = {
  'monthly-statement': `<!DOCTYPE html>
<html>
<head>
    <title>Monthly Statement - {{ account.alias }}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background: #f5f5f5; padding: 20px; margin-bottom: 20px; }
        .transaction { border-bottom: 1px solid #eee; padding: 10px 0; }
    </style>
</head>
<body>
    <div class="header">
        <h1>Monthly Account Statement</h1>
        <p>Account: {{ account.alias }}</p>
        <p>Statement Period: {{ statement_period }}</p>
        <p>Current Balance: {{ balance.available|scale:balance.scale|currency:"USD" }}</p>
    </div>
    
    <h2>Transaction History</h2>
    {% for transaction in transactions|filter:"status='COMPLETED'" %}
    <div class="transaction">
        <div>{{ transaction.created_at|date:"2006-01-02 15:04" }}</div>
        <div>{{ transaction.description }}</div>
        <div>{{ transaction.amount|scale:transaction.scale|currency:"USD" }}</div>
    </div>
    {% endfor %}
</body>
</html>`,

  'transaction-receipt': `<!DOCTYPE html>
<html>
<head>
    <title>Transaction Receipt</title>
</head>
<body>
    <h1>Transaction Receipt</h1>
    <p><strong>Transaction ID:</strong> {{ transaction.id }}</p>
    <p><strong>Amount:</strong> {{ transaction.amount|currency:"USD" }}</p>
    <p><strong>Date:</strong> {{ transaction.created_at|date:"2006-01-02 15:04:05" }}</p>
    <p><strong>Description:</strong> {{ transaction.description }}</p>
    <p><strong>Status:</strong> {{ transaction.status }}</p>
</body>
</html>`
}
