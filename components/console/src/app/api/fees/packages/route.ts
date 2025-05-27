import { NextRequest, NextResponse } from 'next/server'

// Mock data for fee packages
const mockFeePackages = [
  {
    id: 'pkg-001',
    name: 'Basic Fee Package',
    description: 'Standard fee structure for basic accounts',
    status: 'active',
    type: 'standard',
    fees: [
      {
        id: 'fee-001',
        name: 'Monthly Maintenance',
        type: 'recurring',
        amount: 9.99,
        currency: 'USD',
        frequency: 'monthly'
      },
      {
        id: 'fee-002',
        name: 'Transaction Fee',
        type: 'per_transaction',
        amount: 0.25,
        currency: 'USD'
      }
    ],
    metadata: {
      targetAudience: 'retail',
      tier: 'basic'
    },
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z'
  },
  {
    id: 'pkg-002',
    name: 'Premium Fee Package',
    description: 'Premium fee structure with reduced transaction costs',
    status: 'active',
    type: 'premium',
    fees: [
      {
        id: 'fee-003',
        name: 'Monthly Maintenance',
        type: 'recurring',
        amount: 19.99,
        currency: 'USD',
        frequency: 'monthly'
      },
      {
        id: 'fee-004',
        name: 'Transaction Fee',
        type: 'per_transaction',
        amount: 0.1,
        currency: 'USD'
      }
    ],
    metadata: {
      targetAudience: 'premium',
      tier: 'gold'
    },
    createdAt: '2024-01-10T00:00:00Z',
    updatedAt: '2024-01-10T00:00:00Z'
  },
  {
    id: 'pkg-003',
    name: 'Enterprise Fee Package',
    description: 'Customized fee structure for enterprise clients',
    status: 'active',
    type: 'enterprise',
    fees: [
      {
        id: 'fee-005',
        name: 'Annual Maintenance',
        type: 'recurring',
        amount: 999.99,
        currency: 'USD',
        frequency: 'annual'
      },
      {
        id: 'fee-006',
        name: 'Transaction Fee',
        type: 'per_transaction',
        amount: 0.05,
        currency: 'USD'
      },
      {
        id: 'fee-007',
        name: 'Wire Transfer',
        type: 'fixed',
        amount: 15.0,
        currency: 'USD'
      }
    ],
    metadata: {
      targetAudience: 'enterprise',
      tier: 'platinum',
      negotiable: true
    },
    createdAt: '2024-02-01T00:00:00Z',
    updatedAt: '2024-02-01T00:00:00Z'
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
    const paginatedPackages = mockFeePackages.slice(startIndex, endIndex)

    // Return paginated response
    return NextResponse.json({
      data: paginatedPackages,
      page,
      limit,
      total: mockFeePackages.length,
      totalPages: Math.ceil(mockFeePackages.length / limit)
    })
  } catch (error) {
    return NextResponse.json(
      { error: 'Failed to fetch fee packages' },
      { status: 500 }
    )
  }
}
