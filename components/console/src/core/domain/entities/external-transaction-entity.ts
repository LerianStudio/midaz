export interface ExternalTransactionEntity {
  id: string
  importId: string
  externalId?: string
  sourceSystem: string
  amount: number
  currency: string
  date: string
  description?: string
  referenceNumber?: string
  accountNumber?: string
  accountName?: string
  transactionType: ExternalTransactionType
  status: ExternalTransactionStatus
  metadata: Record<string, any>
  rawData: Record<string, any>
  fingerprint?: string
  embedding?: number[]
  createdAt: string
  updatedAt: string
}

export type ExternalTransactionType =
  | 'debit'
  | 'credit'
  | 'transfer'
  | 'fee'
  | 'adjustment'
  | 'reversal'
  | 'unknown'

export type ExternalTransactionStatus =
  | 'imported'
  | 'validated'
  | 'processed'
  | 'matched'
  | 'exception'
  | 'ignored'

export interface ExternalTransactionFilters {
  importId?: string
  sourceSystem?: string
  amountRange?: {
    min: number
    max: number
  }
  dateRange?: {
    start: string
    end: string
  }
  transactionType?: ExternalTransactionType[]
  status?: ExternalTransactionStatus[]
  hasMatches?: boolean
  hasExceptions?: boolean
  currency?: string[]
  searchText?: string
}

export interface ExternalTransactionSummary {
  totalTransactions: number
  totalAmount: number
  byType: Record<ExternalTransactionType, number>
  byStatus: Record<ExternalTransactionStatus, number>
  byCurrency: Record<
    string,
    {
      count: number
      totalAmount: number
    }
  >
  dateRange: {
    earliest: string
    latest: string
  }
}

export interface CreateExternalTransactionRequest {
  importId: string
  externalId?: string
  sourceSystem: string
  amount: number
  currency: string
  date: string
  description?: string
  referenceNumber?: string
  accountNumber?: string
  accountName?: string
  transactionType: ExternalTransactionType
  metadata?: Record<string, any>
  rawData?: Record<string, any>
}

export interface UpdateExternalTransactionRequest {
  externalId?: string
  description?: string
  referenceNumber?: string
  accountNumber?: string
  accountName?: string
  transactionType?: ExternalTransactionType
  metadata?: Record<string, any>
  status?: ExternalTransactionStatus
}

export interface ExternalTransactionAnalytics {
  volumeTrends: {
    date: string
    count: number
    amount: number
  }[]
  typeDistribution: {
    type: ExternalTransactionType
    count: number
    percentage: number
    totalAmount: number
  }[]
  statusDistribution: {
    status: ExternalTransactionStatus
    count: number
    percentage: number
  }[]
  currencyDistribution: {
    currency: string
    count: number
    totalAmount: number
    percentage: number
  }[]
  averageAmountByType: Record<ExternalTransactionType, number>
  processingMetrics: {
    averageMatchTime: number
    matchSuccessRate: number
    exceptionRate: number
  }
}
