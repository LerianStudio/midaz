export interface BankAccountDto {
  bankCode: string
  branch: string
  number: string
  type: string
  holderName: string
}

export interface AliasDto {
  id: string
  name: string
  type: string
  ledgerId: string
  accountId: string
  metadata?: Record<string, string>
  bankAccount?: BankAccountDto
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

export interface CreateAliasDto {
  name: string
  type: string
  ledgerId: string
  accountId: string
  metadata?: Record<string, string>
  bankAccount?: BankAccountDto
}

export interface UpdateAliasDto {
  name?: string
  type?: string
  metadata?: Record<string, string>
  bankAccount?: BankAccountDto
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
