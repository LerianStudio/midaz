// Fee Package Types
export interface FeePackage {
  id: string
  name: string
  ledgerId: string
  types: CalculationType[]
  waivedAccounts: string[]
  active: boolean
  metadata?: Record<string, any>
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

// Calculation Type
export interface CalculationType {
  priority: number
  type: 'FLAT' | 'PERCENTAGE' | 'MAX_BETWEEN_TYPES'
  from?: TypeOperation[]
  to?: TypeOperation[]
  transactionType?: TransactionType
  calculationType: Calculation[]
}

// Type Operation for account matching
export interface TypeOperation {
  accountId?: string
  anyAccount?: boolean
}

// Transaction Type criteria
export interface TransactionType {
  minValue?: number
  maxValue?: number
  currency?: string
  assetCode?: string
}

// Calculation details
export interface Calculation {
  value?: number
  percentage?: number
  fromTo?: string[]
  origin?: string[]
  target?: string[]
  fromToType?: 'REMAINING' | 'ORIGIN' | 'TARGET'
  refAmount?: 'ORIGINAL' | 'FEES' | 'ORIGIN_FEES'
  algorithmId?: string
}

// Fee Calculation Request
export interface FeeCalculationRequest {
  transactionId?: string
  ledgerId: string
  amount: number
  currency: string
  from: string
  to: string
  packageId?: string
  metadata?: Record<string, any>
}

// Fee Calculation Response
export interface FeeCalculationResponse {
  transactionId?: string
  ledgerId: string
  packageId: string
  calculatedFees: CalculatedFee[]
  totalFees: number
  timestamp: string
}

// Calculated Fee details
export interface CalculatedFee {
  type: string
  amount: number
  from: string
  to: string
  details?: Record<string, any>
}

// Fee Analytics
export interface FeeAnalytics {
  totalRevenue: number
  transactionCount: number
  averageFeeRate: number
  waivedAmount: number
  packageBreakdown: PackageAnalytics[]
  timeSeriesData: TimeSeriesPoint[]
}

export interface PackageAnalytics {
  packageId: string
  packageName: string
  revenue: number
  transactionCount: number
  percentage: number
}

export interface TimeSeriesPoint {
  date: string
  revenue: number
  transactionCount: number
}

// Form Types for UI
export interface CreatePackageFormData {
  name: string
  active: boolean
  waivedAccounts: string[]
  types: CalculationType[]
  metadata?: Record<string, any>
}

export interface UpdatePackageFormData extends Partial<CreatePackageFormData> {}

// Mock data generator types
export interface MockFeePackage extends FeePackage {
  _mock?: boolean
}
