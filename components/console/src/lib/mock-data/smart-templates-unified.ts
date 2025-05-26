/**
 * Unified mock data for Smart Templates plugin
 * Comprehensive data structure with templates, reports, data sources, and analytics
 */

// Core interfaces
export interface Template {
  id: string
  name: string
  description: string
  category:
    | 'financial_reports'
    | 'receipts'
    | 'contracts'
    | 'statements'
    | 'notifications'
    | 'custom'
  tags: string[]
  status: 'active' | 'inactive' | 'draft'
  fileUrl: string
  fileName: string
  fileSize: number
  content: string
  mappedFields: Record<string, Record<string, string[]>>
  variables: string[]
  usageCount: number
  lastUsed: string
  createdBy: string
  createdAt: string
  updatedAt: string
  version: string
}

export interface Report {
  id: string
  templateId: string
  templateName: string
  status: 'queued' | 'processing' | 'completed' | 'failed' | 'expired'
  format: 'html' | 'pdf' | 'docx' | 'csv' | 'txt' | 'json'
  fileName: string
  fileUrl?: string
  fileSize?: number
  processingTime?: string
  parameters: Record<string, any>
  generatedBy: string
  generatedAt?: string
  startedAt?: string
  completedAt?: string
  expiresAt?: string
  downloadCount: number
  lastDownloaded?: string
  queuePosition?: number
  estimatedCompletion?: string
  error?: string
  logs: GenerationLog[]
}

export interface GenerationLog {
  timestamp: string
  level: 'info' | 'warning' | 'error' | 'success'
  message: string
  details?: string
}

export interface DataSource {
  id: string
  name: string
  type: 'postgresql' | 'mysql' | 'mongodb' | 'api' | 'file'
  description: string
  status: 'connected' | 'disconnected' | 'error'
  connectionString?: string
  tables: DataSourceTable[]
  lastSync: string
  queryCount: number
  recordCount: number
  schema?: Record<string, any>
}

export interface DataSourceTable {
  name: string
  fields: string[]
  recordCount: number
  lastUpdated?: string
}

export interface TemplateAnalytics {
  overview: {
    totalTemplates: number
    activeTemplates: number
    totalReports: number
    completedReports: number
    failedReports: number
    avgProcessingTime: string
    totalFileSize: number
    uniqueUsers: number
  }
  templateUsage: Array<{
    templateId: string
    templateName: string
    usageCount: number
    percentage: number
    avgProcessingTime: string
  }>
  reportGeneration: Array<{
    date: string
    reports: number
    completedReports: number
    failedReports: number
    avgProcessingTime: number
  }>
  formatDistribution: Record<string, number>
  topUsers: Array<{
    user: string
    reportsGenerated: number
    templatesUsed: number
    lastActivity: string
  }>
  performance: Array<{
    date: string
    avgProcessingTime: number
    successRate: number
    errorCount: number
  }>
}

// Mock data
export const mockTemplates: Template[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231c',
    name: 'Monthly Account Statement',
    description:
      'Detailed monthly statement with transaction history and account summary',
    category: 'financial_reports',
    tags: ['monthly', 'statement', 'account', 'financial'],
    status: 'active',
    fileUrl: '/templates/monthly-statement.tpl',
    fileName: 'monthly-statement.tpl',
    fileSize: 15420,
    content: `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Monthly Statement - {{ account.alias }}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; color: #333; }
        .header { text-align: center; border-bottom: 2px solid #2563eb; padding-bottom: 20px; margin-bottom: 30px; }
        .logo { font-size: 24px; font-weight: bold; color: #2563eb; }
        .account-info { background: #f8fafc; padding: 20px; border-radius: 8px; margin: 20px 0; }
        .transactions { margin-top: 30px; }
        table { width: 100%; border-collapse: collapse; margin-top: 15px; }
        th, td { border: 1px solid #e2e8f0; padding: 12px; text-align: left; }
        th { background-color: #f1f5f9; font-weight: 600; }
        .amount { text-align: right; font-family: monospace; }
        .positive { color: #059669; }
        .negative { color: #dc2626; }
        .summary { background: #f0f9ff; padding: 20px; border-radius: 8px; margin-top: 30px; }
        .footer { text-align: center; margin-top: 40px; color: #64748b; font-size: 12px; }
    </style>
</head>
<body>
    <div class="header">
        <div class="logo">MIDAZ BANKING</div>
        <h1>Monthly Account Statement</h1>
        <p>Statement Period: {{ statement.period }}</p>
    </div>
    
    <div class="account-info">
        <h2>Account Information</h2>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 20px;">
            <div>
                <p><strong>Account ID:</strong> {{ account.id }}</p>
                <p><strong>Account Alias:</strong> {{ account.alias }}</p>
                <p><strong>Account Type:</strong> {{ account.type }}</p>
            </div>
            <div>
                <p><strong>Current Balance:</strong> <span class="amount positive">{{ balance.available | currency:balance.currency }}</span></p>
                <p><strong>Available Balance:</strong> <span class="amount">{{ balance.available | currency:balance.currency }}</span></p>
                <p><strong>Statement Date:</strong> {{ current_date | date:"F j, Y" }}</p>
            </div>
        </div>
    </div>
    
    <div class="transactions">
        <h2>Transaction History</h2>
        <p>{{ transactions|length }} transactions in this period</p>
        <table>
            <thead>
                <tr>
                    <th>Date</th>
                    <th>Description</th>
                    <th>Reference</th>
                    <th>Amount</th>
                    <th>Running Balance</th>
                </tr>
            </thead>
            <tbody>
                {% for transaction in transactions %}
                <tr>
                    <td>{{ transaction.created_at | date:"M j, Y" }}</td>
                    <td>{{ transaction.description }}</td>
                    <td>{{ transaction.id | slice:":8" }}</td>
                    <td class="amount {{ transaction.amount > 0 | yesno:'positive,negative' }}">
                        {{ transaction.amount | currency:transaction.currency }}
                    </td>
                    <td class="amount">{{ transaction.running_balance | currency:transaction.currency }}</td>
                </tr>
                {% endfor %}
            </tbody>
        </table>
    </div>
    
    <div class="summary">
        <h2>Summary</h2>
        <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 20px; text-align: center;">
            <div>
                <h3>Total Credits</h3>
                <p class="amount positive">{{ summary.total_credits | currency:balance.currency }}</p>
            </div>
            <div>
                <h3>Total Debits</h3>
                <p class="amount negative">{{ summary.total_debits | currency:balance.currency }}</p>
            </div>
            <div>
                <h3>Net Change</h3>
                <p class="amount">{{ summary.net_change | currency:balance.currency }}</p>
            </div>
        </div>
    </div>
    
    <div class="footer">
        <p>Generated on {{ current_date | date:"F j, Y \\a\\t g:i A" }} | MIDAZ Banking Platform</p>
        <p>For questions about your account, contact support at support@midaz.com</p>
    </div>
</body>
</html>`,
    mappedFields: {
      midaz_onboarding: {
        account: ['id', 'alias', 'type', 'status', 'ledger_id'],
        ledger: ['id', 'name', 'organization_id']
      },
      midaz_transaction: {
        balance: ['available', 'scale', 'account_id', 'currency'],
        transaction: ['id', 'amount', 'description', 'created_at', 'status']
      }
    },
    variables: [
      'account.id',
      'account.alias',
      'account.type',
      'account.status',
      'balance.available',
      'balance.currency',
      'statement.period',
      'transactions',
      'transaction.created_at',
      'transaction.description',
      'transaction.amount',
      'transaction.running_balance',
      'current_date',
      'summary.total_credits',
      'summary.total_debits',
      'summary.net_change'
    ],
    usageCount: 1456,
    lastUsed: '2025-01-01T12:30:00Z',
    createdBy: 'john.doe@company.com',
    createdAt: '2024-11-15T00:00:00Z',
    updatedAt: '2024-12-20T00:00:00Z',
    version: '2.1.0'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231d',
    name: 'Transaction Receipt',
    description:
      'Standard transaction confirmation receipt for payments and transfers',
    category: 'receipts',
    tags: ['transaction', 'receipt', 'confirmation', 'payment'],
    status: 'active',
    fileUrl: '/templates/transaction-receipt.tpl',
    fileName: 'transaction-receipt.tpl',
    fileSize: 8950,
    content: `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Transaction Receipt</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 400px; margin: 0 auto; padding: 20px; }
        .receipt { border: 2px solid #ddd; border-radius: 8px; padding: 20px; }
        .header { text-align: center; border-bottom: 1px solid #eee; padding-bottom: 15px; margin-bottom: 20px; }
        .status { text-align: center; padding: 10px; border-radius: 5px; margin: 15px 0; }
        .success { background-color: #d4edda; color: #155724; }
        .pending { background-color: #fff3cd; color: #856404; }
        .failed { background-color: #f8d7da; color: #721c24; }
        .details { margin: 15px 0; }
        .row { display: flex; justify-content: space-between; margin: 8px 0; }
        .label { font-weight: bold; }
        .amount { font-size: 18px; font-weight: bold; }
        .footer { text-align: center; margin-top: 20px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="receipt">
        <div class="header">
            <h2>TRANSACTION RECEIPT</h2>
            <p>{{ transaction.created_at | date:"F j, Y \\a\\t g:i A" }}</p>
        </div>
        
        <div class="status {{ transaction.status }}">
            <strong>{{ transaction.status | upper }}</strong>
        </div>
        
        <div class="details">
            <div class="row">
                <span class="label">Transaction ID:</span>
                <span>{{ transaction.id }}</span>
            </div>
            <div class="row">
                <span class="label">Amount:</span>
                <span class="amount">{{ transaction.amount | currency:transaction.currency }}</span>
            </div>
            <div class="row">
                <span class="label">From Account:</span>
                <span>{{ source_account.alias }}</span>
            </div>
            <div class="row">
                <span class="label">To Account:</span>
                <span>{{ destination_account.alias }}</span>
            </div>
            <div class="row">
                <span class="label">Description:</span>
                <span>{{ transaction.description }}</span>
            </div>
            {% if transaction.reference %}
            <div class="row">
                <span class="label">Reference:</span>
                <span>{{ transaction.reference }}</span>
            </div>
            {% endif %}
        </div>
        
        <div class="footer">
            <p>Thank you for using MIDAZ</p>
            <p>Keep this receipt for your records</p>
        </div>
    </div>
</body>
</html>`,
    mappedFields: {
      midaz_transaction: {
        transaction: [
          'id',
          'amount',
          'description',
          'created_at',
          'status',
          'reference'
        ],
        operation: ['type', 'source_account_id', 'destination_account_id']
      },
      midaz_onboarding: {
        account: ['id', 'alias', 'name']
      }
    },
    variables: [
      'transaction.id',
      'transaction.amount',
      'transaction.currency',
      'transaction.description',
      'transaction.created_at',
      'transaction.status',
      'source_account.alias',
      'destination_account.alias',
      'transaction.reference'
    ],
    usageCount: 2340,
    lastUsed: '2025-01-01T16:45:00Z',
    createdBy: 'jane.smith@company.com',
    createdAt: '2024-10-20T00:00:00Z',
    updatedAt: '2024-12-18T00:00:00Z',
    version: '1.5.0'
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231e',
    name: 'KYC Document Request',
    description:
      'Customer notification for required KYC documentation submission',
    category: 'notifications',
    tags: ['kyc', 'compliance', 'notification', 'documentation'],
    status: 'active',
    fileUrl: '/templates/kyc-request.tpl',
    fileName: 'kyc-request.tpl',
    fileSize: 6720,
    content: `Subject: Action Required: Complete Your Account Verification

Dear {{ customer.name }},

We hope this message finds you well. As part of our commitment to maintaining the highest security standards and regulatory compliance, we need to complete the verification process for your account.

**Account Details:**
- Account ID: {{ account.id }}
- Account Type: {{ account.type }}
- Registration Date: {{ account.created_at | date:"F j, Y" }}

**Required Documents:**
{% for document in required_documents %}
- {{ document.name }}: {{ document.description }}
{% endfor %}

**Next Steps:**
1. Log into your account at {{ platform.url }}
2. Navigate to Account Settings > Verification
3. Upload the required documents listed above
4. Submit for review

**Important Information:**
- You have {{ verification.deadline_days }} days to complete this process
- Deadline: {{ verification.deadline | date:"F j, Y" }}
- Failure to complete verification may result in account restrictions

If you have any questions or need assistance, please don't hesitate to contact our support team at {{ platform.support_email }} or call {{ platform.support_phone }}.

Thank you for your cooperation and for choosing MIDAZ.

Best regards,
The MIDAZ Compliance Team

---
This is an automated message. Please do not reply directly to this email.
MIDAZ Financial Platform | {{ platform.address }}`,
    mappedFields: {
      midaz_onboarding: {
        account: ['id', 'type', 'created_at'],
        organization: ['name', 'email', 'phone']
      },
      customer_data: {
        customer: ['name', 'email', 'phone'],
        verification: ['status', 'deadline', 'required_documents']
      }
    },
    variables: [
      'customer.name',
      'account.id',
      'account.type',
      'account.created_at',
      'required_documents',
      'document.name',
      'document.description',
      'platform.url',
      'verification.deadline_days',
      'verification.deadline',
      'platform.support_email',
      'platform.support_phone',
      'platform.address'
    ],
    usageCount: 567,
    lastUsed: '2024-12-30T09:20:00Z',
    createdBy: 'compliance@company.com',
    createdAt: '2024-12-01T00:00:00Z',
    updatedAt: '2024-12-25T00:00:00Z',
    version: '1.0.0'
  }
]

export const mockReports: Report[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231f',
    templateId: '01956b69-9102-75b7-8860-3e75c11d231c',
    templateName: 'Monthly Account Statement',
    status: 'completed',
    format: 'pdf',
    fileName: 'monthly-statement-dec-2024.pdf',
    fileUrl: '/reports/monthly-statement-dec-2024.pdf',
    fileSize: 245760,
    processingTime: '2.5s',
    parameters: {
      account_id: '01956b69-9102-75b7-8860-3e75c11d2320',
      account_alias: 'john-checking-001',
      month: '2024-12',
      include_metadata: true,
      locale: 'en-US',
      timezone: 'America/New_York'
    },
    generatedBy: 'john.doe@company.com',
    generatedAt: '2025-01-01T10:15:00Z',
    completedAt: '2025-01-01T10:17:30Z',
    expiresAt: '2025-02-01T00:00:00Z',
    downloadCount: 3,
    lastDownloaded: '2025-01-01T14:30:00Z',
    logs: [
      {
        timestamp: '2025-01-01T10:15:00Z',
        level: 'info',
        message: 'Report generation started',
        details: 'Template loaded successfully'
      },
      {
        timestamp: '2025-01-01T10:15:01Z',
        level: 'info',
        message: 'Data source connected',
        details: 'Connected to midaz_onboarding database'
      },
      {
        timestamp: '2025-01-01T10:15:02Z',
        level: 'info',
        message: 'Template rendered',
        details: 'Template processed with 45 transactions'
      },
      {
        timestamp: '2025-01-01T10:17:30Z',
        level: 'success',
        message: 'Report generation completed',
        details: 'Total processing time: 2.5 seconds'
      }
    ]
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2321',
    templateId: '01956b69-9102-75b7-8860-3e75c11d231d',
    templateName: 'Transaction Receipt',
    status: 'processing',
    format: 'html',
    fileName: 'transaction-receipt-tx-001.html',
    parameters: {
      transaction_id: '01956b69-9102-75b7-8860-3e75c11d2322'
    },
    generatedBy: 'jane.smith@company.com',
    startedAt: '2025-01-01T16:30:00Z',
    queuePosition: 2,
    estimatedCompletion: '2025-01-01T16:32:00Z',
    downloadCount: 0,
    logs: [
      {
        timestamp: '2025-01-01T16:30:00Z',
        level: 'info',
        message: 'Report queued for processing',
        details: 'Position 2 in queue'
      },
      {
        timestamp: '2025-01-01T16:30:30Z',
        level: 'info',
        message: 'Processing started',
        details: 'Template validation successful'
      }
    ]
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d2323',
    templateId: '01956b69-9102-75b7-8860-3e75c11d231e',
    templateName: 'KYC Document Request',
    status: 'failed',
    format: 'txt',
    fileName: 'kyc-request-customer-001.txt',
    parameters: {
      customer_id: '01956b69-9102-75b7-8860-3e75c11d2324',
      verification_type: 'full_kyc'
    },
    generatedBy: 'compliance@company.com',
    startedAt: '2024-12-30T14:20:00Z',
    downloadCount: 0,
    error: 'Failed to fetch customer data: Connection timeout',
    logs: [
      {
        timestamp: '2024-12-30T14:20:00Z',
        level: 'info',
        message: 'Report generation started'
      },
      {
        timestamp: '2024-12-30T14:20:15Z',
        level: 'error',
        message: 'Data source connection failed',
        details: 'Connection timeout after 15 seconds'
      },
      {
        timestamp: '2024-12-30T14:20:15Z',
        level: 'error',
        message: 'Report generation failed',
        details: 'Failed to fetch customer data: Connection timeout'
      }
    ]
  }
]

export const mockDataSources: DataSource[] = [
  {
    id: 'midaz_onboarding',
    name: 'Midaz Onboarding Database',
    type: 'postgresql',
    description:
      'Core onboarding service database with accounts, ledgers, and organizations',
    status: 'connected',
    tables: [
      {
        name: 'account',
        fields: [
          'id',
          'alias',
          'name',
          'status',
          'ledger_id',
          'organization_id',
          'created_at',
          'updated_at'
        ],
        recordCount: 15420,
        lastUpdated: '2025-01-01T12:00:00Z'
      },
      {
        name: 'ledger',
        fields: [
          'id',
          'name',
          'status',
          'organization_id',
          'created_at',
          'updated_at'
        ],
        recordCount: 48,
        lastUpdated: '2025-01-01T12:00:00Z'
      },
      {
        name: 'organization',
        fields: [
          'id',
          'legal_name',
          'doing_business_as',
          'email',
          'phone',
          'country',
          'created_at'
        ],
        recordCount: 12,
        lastUpdated: '2025-01-01T10:00:00Z'
      }
    ],
    lastSync: '2025-01-01T12:00:00Z',
    queryCount: 1250,
    recordCount: 15480
  },
  {
    id: 'midaz_transaction',
    name: 'Midaz Transaction Database',
    type: 'postgresql',
    description:
      'Transaction service database with balances, transactions, and operations',
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
          'created_at',
          'updated_at'
        ],
        recordCount: 15420,
        lastUpdated: '2025-01-01T12:30:00Z'
      },
      {
        name: 'transaction',
        fields: [
          'id',
          'amount',
          'description',
          'status',
          'created_at',
          'updated_at'
        ],
        recordCount: 89340,
        lastUpdated: '2025-01-01T12:30:00Z'
      },
      {
        name: 'operation',
        fields: [
          'id',
          'transaction_id',
          'type',
          'amount',
          'account_id',
          'created_at'
        ],
        recordCount: 178680,
        lastUpdated: '2025-01-01T12:30:00Z'
      }
    ],
    lastSync: '2025-01-01T12:30:00Z',
    queryCount: 3200,
    recordCount: 283440
  }
]

export const mockAnalytics: TemplateAnalytics = {
  overview: {
    totalTemplates: 12,
    activeTemplates: 10,
    totalReports: 4563,
    completedReports: 4401,
    failedReports: 162,
    avgProcessingTime: '3.2s',
    totalFileSize: 1205760, // bytes
    uniqueUsers: 89
  },
  templateUsage: [
    {
      templateId: '01956b69-9102-75b7-8860-3e75c11d231d',
      templateName: 'Transaction Receipt',
      usageCount: 2340,
      percentage: 51.3,
      avgProcessingTime: '1.2s'
    },
    {
      templateId: '01956b69-9102-75b7-8860-3e75c11d231c',
      templateName: 'Monthly Account Statement',
      usageCount: 1456,
      percentage: 31.9,
      avgProcessingTime: '2.5s'
    },
    {
      templateId: '01956b69-9102-75b7-8860-3e75c11d231e',
      templateName: 'KYC Document Request',
      usageCount: 567,
      percentage: 12.4,
      avgProcessingTime: '0.8s'
    }
  ],
  reportGeneration: [
    {
      date: '2025-01-01',
      reports: 156,
      completedReports: 152,
      failedReports: 4,
      avgProcessingTime: 2.8
    },
    {
      date: '2024-12-31',
      reports: 134,
      completedReports: 131,
      failedReports: 3,
      avgProcessingTime: 3.1
    },
    {
      date: '2024-12-30',
      reports: 189,
      completedReports: 185,
      failedReports: 4,
      avgProcessingTime: 2.9
    },
    {
      date: '2024-12-29',
      reports: 167,
      completedReports: 163,
      failedReports: 4,
      avgProcessingTime: 3.0
    },
    {
      date: '2024-12-28',
      reports: 145,
      completedReports: 140,
      failedReports: 5,
      avgProcessingTime: 3.2
    },
    {
      date: '2024-12-27',
      reports: 98,
      completedReports: 95,
      failedReports: 3,
      avgProcessingTime: 2.7
    },
    {
      date: '2024-12-26',
      reports: 87,
      completedReports: 84,
      failedReports: 3,
      avgProcessingTime: 2.5
    }
  ],
  formatDistribution: {
    pdf: 45.2,
    html: 28.7,
    docx: 15.1,
    csv: 8.3,
    txt: 2.1,
    json: 0.6
  },
  topUsers: [
    {
      user: 'john.doe@company.com',
      reportsGenerated: 567,
      templatesUsed: 8,
      lastActivity: '2025-01-01T16:30:00Z'
    },
    {
      user: 'jane.smith@company.com',
      reportsGenerated: 445,
      templatesUsed: 6,
      lastActivity: '2025-01-01T14:20:00Z'
    },
    {
      user: 'bob.wilson@company.com',
      reportsGenerated: 334,
      templatesUsed: 5,
      lastActivity: '2024-12-31T18:45:00Z'
    }
  ],
  performance: [
    {
      date: '2025-01-01',
      avgProcessingTime: 2.8,
      successRate: 97.4,
      errorCount: 4
    },
    {
      date: '2024-12-31',
      avgProcessingTime: 3.1,
      successRate: 97.8,
      errorCount: 3
    },
    {
      date: '2024-12-30',
      avgProcessingTime: 2.9,
      successRate: 97.9,
      errorCount: 4
    },
    {
      date: '2024-12-29',
      avgProcessingTime: 3.0,
      successRate: 97.6,
      errorCount: 4
    },
    {
      date: '2024-12-28',
      avgProcessingTime: 3.2,
      successRate: 96.6,
      errorCount: 5
    }
  ]
}

// Helper functions
export function getTemplateById(id: string): Template | undefined {
  return mockTemplates.find((template) => template.id === id)
}

export function getReportById(id: string): Report | undefined {
  return mockReports.find((report) => report.id === id)
}

export function getDataSourceById(id: string): DataSource | undefined {
  return mockDataSources.find((source) => source.id === id)
}

export function getReportsByTemplateId(templateId: string): Report[] {
  return mockReports.filter((report) => report.templateId === templateId)
}

export function getTemplatesByCategory(
  category: Template['category']
): Template[] {
  return mockTemplates.filter((template) => template.category === category)
}

export function getActiveTemplates(): Template[] {
  return mockTemplates.filter((template) => template.status === 'active')
}

export function getRecentReports(limit: number = 10): Report[] {
  return mockReports
    .sort(
      (a, b) =>
        new Date(b.generatedAt || b.startedAt || '').getTime() -
        new Date(a.generatedAt || a.startedAt || '').getTime()
    )
    .slice(0, limit)
}
