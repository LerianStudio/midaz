import { NextRequest, NextResponse } from 'next/server'

// Mock data for account types
const mockAccountTypes = [
  {
    id: 'at-001',
    keyValue: 'ASSET_CASH',
    name: 'Cash Account',
    description: 'Physical cash and cash equivalents',
    category: 'asset',
    nature: 'debit',
    domain: 'system',
    status: 'active',
    metadata: {
      reportingCode: '1010',
      taxCategory: 'current_asset'
    },
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z'
  },
  {
    id: 'at-002',
    keyValue: 'ASSET_BANK',
    name: 'Bank Account',
    description: 'Bank deposits and checking accounts',
    category: 'asset',
    nature: 'debit',
    domain: 'system',
    status: 'active',
    metadata: {
      reportingCode: '1020',
      taxCategory: 'current_asset'
    },
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z'
  },
  {
    id: 'at-003',
    keyValue: 'LIABILITY_PAYABLE',
    name: 'Accounts Payable',
    description: 'Amounts owed to suppliers',
    category: 'liability',
    nature: 'credit',
    domain: 'system',
    status: 'active',
    metadata: {
      reportingCode: '2010',
      taxCategory: 'current_liability'
    },
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z'
  },
  {
    id: 'at-004',
    keyValue: 'REVENUE_SALES',
    name: 'Sales Revenue',
    description: 'Revenue from sales of goods or services',
    category: 'revenue',
    nature: 'credit',
    domain: 'system',
    status: 'active',
    metadata: {
      reportingCode: '4010',
      taxCategory: 'operating_revenue'
    },
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z'
  },
  {
    id: 'at-005',
    keyValue: 'EXPENSE_OPERATING',
    name: 'Operating Expenses',
    description: 'General operating expenses',
    category: 'expense',
    nature: 'debit',
    domain: 'system',
    status: 'active',
    metadata: {
      reportingCode: '5010',
      taxCategory: 'operating_expense'
    },
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z'
  },
  {
    id: 'at-006',
    keyValue: 'EQUITY_CAPITAL',
    name: 'Capital Stock',
    description: 'Shareholders equity capital',
    category: 'equity',
    nature: 'credit',
    domain: 'system',
    status: 'active',
    metadata: {
      reportingCode: '3010',
      taxCategory: 'equity'
    },
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z'
  }
]

export async function GET(request: NextRequest) {
  try {
    // Get query parameters
    const searchParams = request.nextUrl.searchParams
    const page = parseInt(searchParams.get('page') || '1')
    const limit = parseInt(searchParams.get('limit') || '10')

    // Calculate pagination
    const startIndex = (page - 1) * limit
    const endIndex = startIndex + limit
    const paginatedAccountTypes = mockAccountTypes.slice(startIndex, endIndex)

    // Return paginated response
    return NextResponse.json({
      data: paginatedAccountTypes,
      page,
      limit,
      total: mockAccountTypes.length,
      totalPages: Math.ceil(mockAccountTypes.length / limit)
    })
  } catch (error) {
    return NextResponse.json(
      { error: 'Failed to fetch account types' },
      { status: 500 }
    )
  }
}
