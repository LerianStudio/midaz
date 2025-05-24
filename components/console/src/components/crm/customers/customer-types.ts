export enum CustomerType {
  NATURAL_PERSON = 'NATURAL_PERSON',
  LEGAL_PERSON = 'LEGAL_PERSON'
}

export interface Address {
  line1: string
  line2?: string
  city: string
  state: string
  zipCode: string
  country: string
}

export interface Contact {
  primaryEmail: string
  secondaryEmail?: string
  mobilePhone: string
  homePhone?: string
}

export interface NaturalPerson {
  birthDate: string
  gender: string
  civilStatus: string
  nationality: string
}

export interface LegalPersonRepresentative {
  name: string
  document: string
  email: string
  role: string
}

export interface LegalPerson {
  tradeName: string
  activity: string
  type: string
  foundingDate: string
  size: string
  representative?: LegalPersonRepresentative
}

export interface CustomerMetadata {
  customerSince: string
  riskLevel: string
  preferredLanguage: string
}

export interface Customer {
  id: string
  type: CustomerType
  name: string
  document: string
  externalId?: string
  status: string
  contact: Contact
  addresses: {
    primary: Address
  }
  naturalPerson?: NaturalPerson
  legalPerson?: LegalPerson
  metadata: CustomerMetadata
  createdAt: string
  updatedAt: string
}

export interface BankingDetails {
  bankId: string
  branch: string
  account: string
  type: string
  iban?: string
  countryCode: string
}

export interface Alias {
  id: string
  holderId: string
  ledgerId: string
  accountId: string
  status: string
  bankingDetails: BankingDetails
  createdAt: string
  updatedAt: string
}
