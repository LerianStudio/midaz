'use client'

import { useState, useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'
import {
  ArrowLeft,
  Eye,
  Download,
  RefreshCw,
  Settings,
  FileText,
  Database,
  Zap
} from 'lucide-react'
import Link from 'next/link'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'

// Mock data for templates and sample data
const mockTemplate = {
  id: '01956b69-9102-75b7-8860-3e75c11d231c',
  name: 'Monthly Account Statement',
  description: 'Detailed monthly statement with transaction history',
  category: 'financial_reports',
  status: 'active',
  content: `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Account Statement - {{ account.alias }}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .header { text-align: center; border-bottom: 2px solid #333; padding-bottom: 20px; }
        .account-info { margin: 20px 0; }
        .transactions { margin-top: 30px; }
        table { width: 100%; border-collapse: collapse; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .balance { text-align: right; font-weight: bold; }
    </style>
</head>
<body>
    <div class="header">
        <h1>Monthly Account Statement</h1>
        <p>Statement Period: {{ statement.period }}</p>
    </div>
    
    <div class="account-info">
        <h2>Account Information</h2>
        <p><strong>Account ID:</strong> {{ account.id }}</p>
        <p><strong>Account Alias:</strong> {{ account.alias }}</p>
        <p><strong>Account Type:</strong> {{ account.type }}</p>
        <p><strong>Current Balance:</strong> {{ balance.available | currency:balance.currency }}</p>
    </div>
    
    <div class="transactions">
        <h2>Transaction History</h2>
        <table>
            <thead>
                <tr>
                    <th>Date</th>
                    <th>Description</th>
                    <th>Amount</th>
                    <th>Balance</th>
                </tr>
            </thead>
            <tbody>
                {% for transaction in transactions %}
                <tr>
                    <td>{{ transaction.created_at | date:"Y-m-d H:i" }}</td>
                    <td>{{ transaction.description }}</td>
                    <td class="balance">{{ transaction.amount | currency:transaction.currency }}</td>
                    <td class="balance">{{ transaction.running_balance | currency:transaction.currency }}</td>
                </tr>
                {% endfor %}
            </tbody>
        </table>
    </div>
    
    <div style="margin-top: 40px; text-align: center; color: #666;">
        <p>Generated on {{ current_date | date:"F j, Y \\a\\t g:i A" }}</p>
    </div>
</body>
</html>`
}

const mockSampleData = {
  account: {
    id: '01956b69-9102-75b7-8860-3e75c11d231f',
    alias: 'john-checking-001',
    type: 'CHECKING',
    status: 'active'
  },
  balance: {
    available: 2540.75,
    currency: 'USD'
  },
  statement: {
    period: 'December 2024'
  },
  transactions: [
    {
      id: 'tx-001',
      description: 'Direct Deposit - Salary',
      amount: 3500.0,
      currency: 'USD',
      running_balance: 2540.75,
      created_at: '2024-12-31T09:00:00Z'
    },
    {
      id: 'tx-002',
      description: 'ATM Withdrawal',
      amount: -200.0,
      currency: 'USD',
      running_balance: 2740.75,
      created_at: '2024-12-30T14:30:00Z'
    },
    {
      id: 'tx-003',
      description: 'Online Purchase - Amazon',
      amount: -89.99,
      currency: 'USD',
      running_balance: 2940.75,
      created_at: '2024-12-29T16:45:00Z'
    },
    {
      id: 'tx-004',
      description: 'Restaurant Payment',
      amount: -45.2,
      currency: 'USD',
      running_balance: 3030.74,
      created_at: '2024-12-28T19:15:00Z'
    }
  ],
  current_date: '2025-01-01T00:00:00Z'
}

const sampleDataSets = [
  {
    id: 'default',
    name: 'Default Sample Data',
    description: 'Standard account with transactions'
  },
  {
    id: 'business',
    name: 'Business Account',
    description: 'High volume business account'
  },
  {
    id: 'savings',
    name: 'Savings Account',
    description: 'Interest-bearing savings account'
  },
  {
    id: 'empty',
    name: 'Empty Account',
    description: 'Account with no transactions'
  }
]

export default function TemplatePreviewPage() {
  const params = useParams()
  const router = useRouter()
  const templateId = params.id as string

  const [isLoading, setIsLoading] = useState(true)
  const [selectedDataSet, setSelectedDataSet] = useState('default')
  const [previewFormat, setPreviewFormat] = useState('html')
  const [isGenerating, setIsGenerating] = useState(false)

  useEffect(() => {
    // Simulate loading
    const timer = setTimeout(() => setIsLoading(false), 1000)
    return () => clearTimeout(timer)
  }, [])

  const generatePreview = async () => {
    setIsGenerating(true)
    // Simulate template processing
    await new Promise((resolve) => setTimeout(resolve, 2000))
    setIsGenerating(false)
  }

  const renderPreviewContent = () => {
    if (previewFormat === 'html') {
      // Simple template rendering simulation
      const rendered = mockTemplate.content
        .replace(/\{\{\s*account\.alias\s*\}\}/g, mockSampleData.account.alias)
        .replace(/\{\{\s*account\.id\s*\}\}/g, mockSampleData.account.id)
        .replace(/\{\{\s*account\.type\s*\}\}/g, mockSampleData.account.type)
        .replace(
          /\{\{\s*statement\.period\s*\}\}/g,
          mockSampleData.statement.period
        )
        .replace(
          /\{\{\s*balance\.available.*?\}\}/g,
          `$${mockSampleData.balance.available.toFixed(2)}`
        )
        .replace(/\{\{\s*current_date.*?\}\}/g, 'January 1, 2025 at 12:00 AM')

      return (
        <div className="rounded-lg border bg-white p-4">
          <iframe
            srcDoc={rendered}
            className="h-[600px] w-full border-0"
            title="Template Preview"
          />
        </div>
      )
    }

    return (
      <div className="rounded-lg border bg-gray-50 p-4">
        <div className="py-8 text-center">
          <FileText className="mx-auto mb-4 h-12 w-12 text-gray-400" />
          <p className="text-gray-600">
            {previewFormat.toUpperCase()} preview will be available here
          </p>
          <p className="mt-2 text-sm text-gray-500">
            Select HTML format for live preview
          </p>
        </div>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader.Root>
          <div className="flex items-center gap-3">
            <Skeleton className="h-8 w-8" />
            <div className="space-y-2">
              <Skeleton className="h-6 w-48" />
              <Skeleton className="h-4 w-64" />
            </div>
          </div>
          <Skeleton className="h-8 w-32" />
        </PageHeader.Root>
        <div className="space-y-4">
          <Skeleton className="h-32" />
          <Skeleton className="h-96" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" asChild>
            <Link href={`/plugins/smart-templates/templates/${templateId}`}>
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <PageHeader.InfoTitle
              title="Template Preview"
              subtitle={mockTemplate.name}
            />
            <div className="mt-2 flex items-center gap-2">
              <Badge variant="secondary">
                {mockTemplate.category.replace('_', ' ')}
              </Badge>
              <Badge
                variant={
                  mockTemplate.status === 'active' ? 'default' : 'secondary'
                }
              >
                {mockTemplate.status}
              </Badge>
            </div>
          </div>
        </div>
        <PageHeader.InfoTooltip subtitle="Preview template with sample data in different formats" />
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={generatePreview}
            disabled={isGenerating}
          >
            {isGenerating ? (
              <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-4 w-4" />
            )}
            Regenerate
          </Button>
          <Button>
            <Download className="mr-2 h-4 w-4" />
            Download
          </Button>
        </div>
      </PageHeader.Root>

      {/* Preview Controls */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Database className="h-4 w-4" />
              Sample Data
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Select value={selectedDataSet} onValueChange={setSelectedDataSet}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {sampleDataSets.map((dataSet) => (
                  <SelectItem key={dataSet.id} value={dataSet.id}>
                    <div>
                      <div className="font-medium">{dataSet.name}</div>
                      <div className="text-xs text-gray-500">
                        {dataSet.description}
                      </div>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Eye className="h-4 w-4" />
              Preview Format
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Select value={previewFormat} onValueChange={setPreviewFormat}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="html">HTML</SelectItem>
                <SelectItem value="pdf">PDF</SelectItem>
                <SelectItem value="docx">DOCX</SelectItem>
                <SelectItem value="txt">Plain Text</SelectItem>
              </SelectContent>
            </Select>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Settings className="h-4 w-4" />
              Options
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              <div className="text-xs text-gray-600">
                Real-time updates: Enabled
              </div>
              <div className="text-xs text-gray-600">
                Last updated: Just now
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Preview Tabs */}
      <Tabs defaultValue="preview" className="w-full">
        <TabsList>
          <TabsTrigger value="preview">Preview</TabsTrigger>
          <TabsTrigger value="data">Sample Data</TabsTrigger>
          <TabsTrigger value="variables">Variables</TabsTrigger>
        </TabsList>

        <TabsContent value="preview" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Eye className="h-5 w-5" />
                Template Preview
              </CardTitle>
              <CardDescription>
                Live preview of the template with selected sample data
              </CardDescription>
            </CardHeader>
            <CardContent>{renderPreviewContent()}</CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="data" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Database className="h-5 w-5" />
                Sample Data Structure
              </CardTitle>
              <CardDescription>
                JSON representation of the data used for preview
              </CardDescription>
            </CardHeader>
            <CardContent>
              <pre className="overflow-x-auto rounded-lg bg-gray-100 p-4 text-sm">
                {JSON.stringify(mockSampleData, null, 2)}
              </pre>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="variables" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Zap className="h-5 w-5" />
                Available Variables
              </CardTitle>
              <CardDescription>
                Variables and filters available in this template
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div>
                  <h4 className="mb-2 font-medium">Account Variables</h4>
                  <div className="grid grid-cols-2 gap-2 text-sm">
                    <code className="rounded bg-gray-100 p-1">account.id</code>
                    <code className="rounded bg-gray-100 p-1">
                      account.alias
                    </code>
                    <code className="rounded bg-gray-100 p-1">
                      account.type
                    </code>
                    <code className="rounded bg-gray-100 p-1">
                      account.status
                    </code>
                  </div>
                </div>
                <div>
                  <h4 className="mb-2 font-medium">Balance Variables</h4>
                  <div className="grid grid-cols-2 gap-2 text-sm">
                    <code className="rounded bg-gray-100 p-1">
                      balance.available
                    </code>
                    <code className="rounded bg-gray-100 p-1">
                      balance.currency
                    </code>
                  </div>
                </div>
                <div>
                  <h4 className="mb-2 font-medium">Available Filters</h4>
                  <div className="grid grid-cols-2 gap-2 text-sm">
                    <code className="rounded bg-gray-100 p-1">| currency</code>
                    <code className="rounded bg-gray-100 p-1">| date</code>
                    <code className="rounded bg-gray-100 p-1">| format</code>
                    <code className="rounded bg-gray-100 p-1">| upper</code>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
