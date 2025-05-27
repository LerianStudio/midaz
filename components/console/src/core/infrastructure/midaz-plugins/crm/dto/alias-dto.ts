export interface BankingDetailsDto {
  branch: string
  account: string
  type: string
  countryCode: string
  bankId: string
}

export interface AliasDto {
  id: string
  document: string
  type: string
  ledgerId: string
  accountId: string
  holderId: string
  metadata?: Record<string, string>
  bankingDetails?: BankingDetailsDto
  createdAt: string
  updatedAt: string
  deletedAt?: string | null
}

export interface CreateAliasDto {
  document: string
  type: string
  ledgerId: string
  accountId: string
  holderId: string
  metadata?: Record<string, string>
  bankingDetails?: BankingDetailsDto
}

export interface UpdateAliasDto {
  document?: string
  type?: string
  metadata?: Record<string, string>
  bankingDetails?: BankingDetailsDto
}

export interface AliasPaginatedResponseDto {
  data: AliasDto[]
  pagination: {
    page: number
    limit: number
    total: number
    totalPages: number
  }
}
