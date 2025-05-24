import { v7 as uuidv7 } from 'uuid'
import {
  FeePackage,
  CalculationType,
  FeeCalculationResponse,
  FeeAnalytics,
  PackageAnalytics,
  TimeSeriesPoint
} from '../types/fee-types'

// Mock fee packages
export const mockFeePackages: FeePackage[] = [
  {
    id: uuidv7(),
    name: 'Standard Transaction Fees',
    ledgerId: 'main-ledger',
    active: true,
    types: [
      {
        priority: 1,
        type: 'PERCENTAGE',
        from: [{ anyAccount: true }],
        to: [{ anyAccount: true }],
        transactionType: {
          minValue: 100,
          currency: 'USD'
        },
        calculationType: [
          {
            percentage: 2.5,
            refAmount: 'ORIGINAL',
            origin: ['fees-revenue'],
            target: ['merchant-account']
          }
        ]
      }
    ],
    waivedAccounts: ['vip-account-123', 'vip-account-456'],
    metadata: {
      category: 'standard',
      approvedBy: 'admin@company.com',
      description: 'Standard fee structure for regular transactions'
    },
    createdAt: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
    updatedAt: new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString()
  },
  {
    id: uuidv7(),
    name: 'Premium Merchant Fees',
    ledgerId: 'main-ledger',
    active: true,
    types: [
      {
        priority: 1,
        type: 'FLAT',
        from: [{ anyAccount: true }],
        to: [{ accountId: 'merchant-premium' }],
        calculationType: [
          {
            value: 0.3,
            fromTo: ['fees-fixed'],
            fromToType: 'ORIGIN'
          }
        ]
      },
      {
        priority: 2,
        type: 'PERCENTAGE',
        from: [{ anyAccount: true }],
        to: [{ accountId: 'merchant-premium' }],
        calculationType: [
          {
            percentage: 1.5,
            refAmount: 'FEES',
            origin: ['fees-percentage'],
            target: ['merchant-premium']
          }
        ]
      }
    ],
    waivedAccounts: [],
    metadata: {
      category: 'premium',
      tier: 'gold'
    },
    createdAt: new Date(Date.now() - 60 * 24 * 60 * 60 * 1000).toISOString(),
    updatedAt: new Date(Date.now() - 5 * 24 * 60 * 60 * 1000).toISOString()
  },
  {
    id: uuidv7(),
    name: 'International Transfer Fees',
    ledgerId: 'main-ledger',
    active: true,
    types: [
      {
        priority: 1,
        type: 'MAX_BETWEEN_TYPES',
        transactionType: {
          minValue: 1000,
          currency: 'USD'
        },
        calculationType: [
          {
            value: 25,
            fromTo: ['fees-international'],
            fromToType: 'ORIGIN'
          },
          {
            percentage: 3.5,
            refAmount: 'ORIGINAL',
            origin: ['fees-international-percentage'],
            target: ['international-settlements']
          }
        ]
      }
    ],
    waivedAccounts: ['diplomatic-account-001'],
    metadata: {
      category: 'international',
      regions: ['US', 'EU', 'ASIA']
    },
    createdAt: new Date(Date.now() - 90 * 24 * 60 * 60 * 1000).toISOString(),
    updatedAt: new Date(Date.now() - 10 * 24 * 60 * 60 * 1000).toISOString()
  },
  {
    id: uuidv7(),
    name: 'Micro-transaction Fees',
    ledgerId: 'main-ledger',
    active: false,
    types: [
      {
        priority: 1,
        type: 'FLAT',
        transactionType: {
          maxValue: 10,
          currency: 'USD'
        },
        calculationType: [
          {
            value: 0.1,
            fromTo: ['fees-micro'],
            fromToType: 'ORIGIN'
          }
        ]
      }
    ],
    waivedAccounts: [],
    metadata: {
      category: 'micro',
      deprecated: true,
      deprecationReason: 'Replaced by dynamic pricing model'
    },
    createdAt: new Date(Date.now() - 180 * 24 * 60 * 60 * 1000).toISOString(),
    updatedAt: new Date(Date.now() - 90 * 24 * 60 * 60 * 1000).toISOString()
  }
]

// Generate mock calculation response
export function generateMockCalculation(
  amount: number,
  packageId: string,
  from: string,
  to: string
): FeeCalculationResponse {
  const selectedPackage =
    mockFeePackages.find((p) => p.id === packageId) || mockFeePackages[0]
  const calculatedFees: any[] = []
  let totalFees = 0

  selectedPackage.types.forEach((type) => {
    if (type.type === 'FLAT') {
      const fee = type.calculationType[0].value || 0
      calculatedFees.push({
        type: 'FLAT',
        amount: fee,
        from: from,
        to: type.calculationType[0].fromTo?.[0] || 'fees-account',
        details: { priority: type.priority }
      })
      totalFees += fee
    } else if (type.type === 'PERCENTAGE') {
      const percentage = type.calculationType[0].percentage || 0
      const fee = (amount * percentage) / 100
      calculatedFees.push({
        type: 'PERCENTAGE',
        amount: fee,
        from: from,
        to: type.calculationType[0].target?.[0] || 'fees-account',
        details: {
          priority: type.priority,
          rate: percentage,
          baseAmount: amount
        }
      })
      totalFees += fee
    }
  })

  return {
    transactionId: uuidv7(),
    ledgerId: 'main-ledger',
    packageId: selectedPackage.id,
    calculatedFees,
    totalFees,
    timestamp: new Date().toISOString()
  }
}

// Generate mock analytics data
export function generateMockAnalytics(): FeeAnalytics {
  const packageBreakdown: PackageAnalytics[] = mockFeePackages
    .filter((p) => p.active)
    .map((pkg, index) => ({
      packageId: pkg.id,
      packageName: pkg.name,
      revenue: Math.random() * 50000 + 10000,
      transactionCount: Math.floor(Math.random() * 1000 + 200),
      percentage: [45, 30, 15, 10][index] || 5
    }))

  const timeSeriesData: TimeSeriesPoint[] = []
  for (let i = 29; i >= 0; i--) {
    const date = new Date()
    date.setDate(date.getDate() - i)
    timeSeriesData.push({
      date: date.toISOString().split('T')[0],
      revenue: Math.random() * 5000 + 2000,
      transactionCount: Math.floor(Math.random() * 200 + 50)
    })
  }

  return {
    totalRevenue: packageBreakdown.reduce((sum, pkg) => sum + pkg.revenue, 0),
    transactionCount: packageBreakdown.reduce(
      (sum, pkg) => sum + pkg.transactionCount,
      0
    ),
    averageFeeRate: 2.35,
    waivedAmount: 4320.5,
    packageBreakdown,
    timeSeriesData
  }
}

// Helper functions for mock data
export function getPackageById(id: string): FeePackage | undefined {
  return mockFeePackages.find((pkg) => pkg.id === id)
}

export function getActivePackages(): FeePackage[] {
  return mockFeePackages.filter((pkg) => pkg.active)
}

export function searchPackages(query: string): FeePackage[] {
  const lowercaseQuery = query.toLowerCase()
  return mockFeePackages.filter(
    (pkg) =>
      pkg.name.toLowerCase().includes(lowercaseQuery) ||
      pkg.metadata?.category?.toLowerCase().includes(lowercaseQuery)
  )
}

// Generate sample transactions for testing
export function generateSampleTransactions() {
  return [
    {
      amount: 100,
      description: 'Small purchase',
      from: 'customer-001',
      to: 'merchant-001'
    },
    {
      amount: 1500,
      description: 'Medium transaction',
      from: 'customer-002',
      to: 'merchant-002'
    },
    {
      amount: 10000,
      description: 'Large transfer',
      from: 'business-001',
      to: 'business-002'
    },
    {
      amount: 50,
      description: 'Micro payment',
      from: 'user-001',
      to: 'service-001'
    },
    {
      amount: 5000,
      description: 'International wire',
      from: 'company-001',
      to: 'company-intl-001'
    }
  ]
}
