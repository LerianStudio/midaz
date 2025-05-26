export interface AddressDto {
  line1: string
  line2?: string
  zipCode: string
  city: string
  state: string
  country: string
}

export interface ContactDto {
  name: string
  value: string
}

export interface HolderDto {
  id: string
  name: string
  status: string
  type: 'NATURAL_PERSON' | 'LEGAL_PERSON'
  document: string
  address?: AddressDto
  tradingName?: string
  legalName?: string
  website?: string
  establishedOn?: string
  monthlyIncomeTotal?: number
  contacts?: ContactDto[]
  metadata?: Record<string, string>
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

export interface CreateHolderDto {
  name: string
  type: 'NATURAL_PERSON' | 'LEGAL_PERSON'
  document: string
  status?: string
  address?: AddressDto
  tradingName?: string
  legalName?: string
  website?: string
  establishedOn?: string
  monthlyIncomeTotal?: number
  contacts?: ContactDto[]
  metadata?: Record<string, string>
}

export interface UpdateHolderDto {
  name?: string
  status?: string
  address?: AddressDto
  tradingName?: string
  legalName?: string
  website?: string
  establishedOn?: string
  monthlyIncomeTotal?: number
  contacts?: ContactDto[]
  metadata?: Record<string, string>
}

export interface HolderPaginatedResponseDto {
  data: HolderDto[]
  pagination: {
    page: number
    limit: number
    total: number
    totalPages: number
  }
}
