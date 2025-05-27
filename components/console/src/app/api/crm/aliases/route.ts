import { NextRequest, NextResponse } from 'next/server'

// Mock data for aliases across all holders
const mockAliases = [
  {
    id: 'alias-001',
    holderId: 'holder-001',
    name: 'Primary Business Account',
    type: 'bank_account',
    accountId: 'acc-001',
    ledgerId: 'ledger-001',
    metadata: {
      bankName: 'Commercial Bank',
      accountNumber: '****1234'
    },
    createdAt: '2024-01-15T10:00:00Z',
    updatedAt: '2024-01-15T10:00:00Z'
  },
  {
    id: 'alias-002',
    holderId: 'holder-001',
    name: 'Savings Account',
    type: 'bank_account',
    accountId: 'acc-002',
    ledgerId: 'ledger-001',
    metadata: {
      bankName: 'Savings Bank',
      accountNumber: '****5678'
    },
    createdAt: '2024-01-20T10:00:00Z',
    updatedAt: '2024-01-20T10:00:00Z'
  },
  {
    id: 'alias-003',
    holderId: 'holder-002',
    name: 'Corporate Account',
    type: 'bank_account',
    accountId: 'acc-003',
    ledgerId: 'ledger-001',
    metadata: {
      bankName: 'Business Bank',
      accountNumber: '****9012'
    },
    createdAt: '2024-02-01T10:00:00Z',
    updatedAt: '2024-02-01T10:00:00Z'
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
    const paginatedAliases = mockAliases.slice(startIndex, endIndex)

    // Return paginated response
    return NextResponse.json({
      data: paginatedAliases,
      page,
      limit,
      total: mockAliases.length,
      totalPages: Math.ceil(mockAliases.length / limit)
    })
  } catch (error) {
    return NextResponse.json(
      { error: 'Failed to fetch aliases' },
      { status: 500 }
    )
  }
}
