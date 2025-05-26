import { Customer, CustomerType, Alias } from './customer-types'

// Helper function to generate random UUIDs
function generateUUID(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

// Helper function to generate random dates
function randomDate(start: Date, end: Date): Date {
  return new Date(
    start.getTime() + Math.random() * (end.getTime() - start.getTime())
  )
}

// Mock data generators
const naturalPersonNames = [
  'Maria Santos Silva',
  'João Silva Santos',
  'Ana Paula Costa',
  'Carlos Eduardo Lima',
  'Fernanda Oliveira',
  'Ricardo Almeida',
  'Patrícia Rocha',
  'Marcos Vinícius',
  'Juliana Ferreira',
  'Rafael Cardoso',
  'Camila Ribeiro',
  'Thiago Monteiro',
  'Bianca Araújo',
  'Diego Pereira',
  'Larissa Gomes',
  'Felipe Barbosa',
  'Natália Castro',
  'Bruno Martins',
  'Gabriela Souza',
  'Leonardo Dias'
]

const legalPersonNames = [
  'TechCorp Solutions Ltda',
  'InnovateBank Corp',
  'Digital Finance S.A.',
  'CloudTech Sistemas',
  'DataFlow Tecnologia',
  'FinanceFlow Ltda',
  'SmartPay Solutions',
  'NextGen Banking',
  'SecureTransact Corp',
  'CryptoSafe Ltda',
  'BlockChain Finance',
  'FutureBank S.A.',
  'PaymentHub Corp',
  'TransferWise Ltda',
  'DigitalWallet Inc',
  'MoneyFlow Systems',
  'BankingTech S.A.',
  'FinTech Solutions',
  'PaySafe Corporation',
  'SwiftTransfer Ltda'
]

const cities = [
  { city: 'São Paulo', state: 'SP', country: 'BR' },
  { city: 'Rio de Janeiro', state: 'RJ', country: 'BR' },
  { city: 'Brasília', state: 'DF', country: 'BR' },
  { city: 'Salvador', state: 'BA', country: 'BR' },
  { city: 'Fortaleza', state: 'CE', country: 'BR' },
  { city: 'Belo Horizonte', state: 'MG', country: 'BR' },
  { city: 'Manaus', state: 'AM', country: 'BR' },
  { city: 'Curitiba', state: 'PR', country: 'BR' },
  { city: 'Recife', state: 'PE', country: 'BR' },
  { city: 'Porto Alegre', state: 'RS', country: 'BR' }
]

const emailDomains = [
  'gmail.com',
  'yahoo.com',
  'hotmail.com',
  'outlook.com',
  'empresa.com.br'
]

const statuses = ['active', 'pending', 'inactive']
const genders = ['Male', 'Female', 'Other']
const civilStatuses = ['Single', 'Married', 'Divorced', 'Widowed']
const companyTypes = [
  'Limited Liability Company',
  'Corporation',
  'Partnership',
  'Sole Proprietorship'
]
const companySizes = ['Micro', 'Small', 'Medium', 'Large']
const activities = [
  'Software Development',
  'Financial Services',
  'Consulting',
  'E-commerce',
  'Manufacturing',
  'Healthcare',
  'Education',
  'Real Estate',
  'Retail',
  'Technology'
]

export function generateMockCustomers(count: number): Customer[] {
  const customers: Customer[] = []

  for (let i = 0; i < count; i++) {
    const isNaturalPerson = Math.random() > 0.3 // 70% natural persons, 30% legal persons
    const customerType = isNaturalPerson
      ? CustomerType.NATURAL_PERSON
      : CustomerType.LEGAL_PERSON
    const location = cities[Math.floor(Math.random() * cities.length)]
    const createdAt = randomDate(new Date(2023, 0, 1), new Date())

    let customer: Customer

    if (isNaturalPerson) {
      const name =
        naturalPersonNames[
          Math.floor(Math.random() * naturalPersonNames.length)
        ]
      const firstName = name.split(' ')[0].toLowerCase()

      customer = {
        id: generateUUID(),
        type: customerType,
        name,
        document: `${Math.floor(Math.random() * 900 + 100)}.${Math.floor(Math.random() * 900 + 100)}.${Math.floor(Math.random() * 900 + 100)}-${Math.floor(Math.random() * 90 + 10)}`,
        externalId: `CUST_${new Date().getFullYear()}_${String(i + 1).padStart(3, '0')}`,
        status: statuses[Math.floor(Math.random() * statuses.length)],
        contact: {
          primaryEmail: `${firstName}.${Math.floor(Math.random() * 1000)}@${emailDomains[Math.floor(Math.random() * emailDomains.length)]}`,
          secondaryEmail:
            Math.random() > 0.7
              ? `${firstName}.personal@${emailDomains[Math.floor(Math.random() * emailDomains.length)]}`
              : undefined,
          mobilePhone: `+55 11 9${Math.floor(Math.random() * 9000 + 1000)}-${Math.floor(Math.random() * 9000 + 1000)}`,
          homePhone:
            Math.random() > 0.6
              ? `+55 11 3${Math.floor(Math.random() * 900 + 100)}-${Math.floor(Math.random() * 9000 + 1000)}`
              : undefined
        },
        addresses: {
          primary: {
            line1: `Rua ${['das Flores', 'dos Anjos', 'da Paz', 'do Sol', 'das Palmeiras'][Math.floor(Math.random() * 5)]}, ${Math.floor(Math.random() * 999 + 1)}`,
            line2:
              Math.random() > 0.7
                ? `Apt ${Math.floor(Math.random() * 999 + 1)}`
                : undefined,
            city: location.city,
            state: location.state,
            zipCode: `${Math.floor(Math.random() * 90000 + 10000)}-${Math.floor(Math.random() * 900 + 100)}`,
            country: location.country
          }
        },
        naturalPerson: {
          birthDate: randomDate(new Date(1950, 0, 1), new Date(2005, 11, 31))
            .toISOString()
            .split('T')[0],
          gender: genders[Math.floor(Math.random() * genders.length)],
          civilStatus:
            civilStatuses[Math.floor(Math.random() * civilStatuses.length)],
          nationality: 'Brazilian'
        },
        metadata: {
          customerSince: createdAt.toISOString().split('T')[0],
          riskLevel: ['Low', 'Medium', 'High'][Math.floor(Math.random() * 3)],
          preferredLanguage: 'pt-BR'
        },
        createdAt: createdAt.toISOString(),
        updatedAt: randomDate(createdAt, new Date()).toISOString()
      }
    } else {
      const companyName =
        legalPersonNames[Math.floor(Math.random() * legalPersonNames.length)]
      const tradeName = companyName.split(' ')[0]

      customer = {
        id: generateUUID(),
        type: customerType,
        name: companyName,
        document: `${Math.floor(Math.random() * 90 + 10)}.${Math.floor(Math.random() * 900 + 100)}.${Math.floor(Math.random() * 900 + 100)}/0001-${Math.floor(Math.random() * 90 + 10)}`,
        externalId: `CORP_${new Date().getFullYear()}_${String(i + 1).padStart(3, '0')}`,
        status: statuses[Math.floor(Math.random() * statuses.length)],
        contact: {
          primaryEmail: `contact@${tradeName.toLowerCase()}.com`,
          secondaryEmail:
            Math.random() > 0.5
              ? `admin@${tradeName.toLowerCase()}.com`
              : undefined,
          mobilePhone: `+55 11 3${Math.floor(Math.random() * 900 + 100)}-${Math.floor(Math.random() * 9000 + 1000)}`,
          homePhone: undefined
        },
        addresses: {
          primary: {
            line1: `Av. ${['Paulista', 'Faria Lima', 'Brigadeiro', 'Ibirapuera', 'Consolação'][Math.floor(Math.random() * 5)]}, ${Math.floor(Math.random() * 9999 + 1000)}`,
            line2: `${Math.floor(Math.random() * 30 + 1)}º andar, sala ${Math.floor(Math.random() * 99 + 10)}`,
            city: location.city,
            state: location.state,
            zipCode: `${Math.floor(Math.random() * 90000 + 10000)}-${Math.floor(Math.random() * 900 + 100)}`,
            country: location.country
          }
        },
        legalPerson: {
          tradeName,
          activity: activities[Math.floor(Math.random() * activities.length)],
          type: companyTypes[Math.floor(Math.random() * companyTypes.length)],
          foundingDate: randomDate(new Date(1990, 0, 1), new Date(2020, 11, 31))
            .toISOString()
            .split('T')[0],
          size: companySizes[Math.floor(Math.random() * companySizes.length)],
          representative: {
            name: naturalPersonNames[
              Math.floor(Math.random() * naturalPersonNames.length)
            ],
            document: `${Math.floor(Math.random() * 900 + 100)}.${Math.floor(Math.random() * 900 + 100)}.${Math.floor(Math.random() * 900 + 100)}-${Math.floor(Math.random() * 90 + 10)}`,
            email: `ceo@${tradeName.toLowerCase()}.com`,
            role: ['CEO', 'President', 'Director', 'Partner'][
              Math.floor(Math.random() * 4)
            ]
          }
        },
        metadata: {
          customerSince: createdAt.toISOString().split('T')[0],
          riskLevel: ['Low', 'Medium', 'High'][Math.floor(Math.random() * 3)],
          preferredLanguage: 'pt-BR'
        },
        createdAt: createdAt.toISOString(),
        updatedAt: randomDate(createdAt, new Date()).toISOString()
      }
    }

    customers.push(customer)
  }

  return customers
}

export function generateMockAliases(count: number): Alias[] {
  const aliases: Alias[] = []
  const customers = generateMockCustomers(50) // Generate some customers to link to
  const bankIds = [
    '001',
    '033',
    '104',
    '237',
    '341',
    '356',
    '422',
    '633',
    '745'
  ]
  const accountTypes = ['CHECKING', 'SAVINGS', 'BUSINESS']
  const aliasStatuses = ['active', 'pending', 'inactive']

  for (let i = 0; i < count; i++) {
    const customer = customers[Math.floor(Math.random() * customers.length)]
    const bankId = bankIds[Math.floor(Math.random() * bankIds.length)]
    const createdAt = randomDate(new Date(2023, 0, 1), new Date())

    const alias: Alias = {
      id: generateUUID(),
      holderId: customer.id,
      ledgerId: generateUUID(),
      accountId: generateUUID(),
      status: aliasStatuses[Math.floor(Math.random() * aliasStatuses.length)],
      bankingDetails: {
        bankId,
        branch: String(Math.floor(Math.random() * 9999 + 1)).padStart(4, '0'),
        account: `${Math.floor(Math.random() * 999999 + 100000)}-${Math.floor(Math.random() * 9)}`,
        type: accountTypes[Math.floor(Math.random() * accountTypes.length)],
        iban: `BR${String(Math.floor(Math.random() * 99)).padStart(2, '0')}${bankId}${String(Math.floor(Math.random() * 999999999999999999)).padStart(18, '0')}`,
        countryCode: 'BR'
      },
      createdAt: createdAt.toISOString(),
      updatedAt: randomDate(createdAt, new Date()).toISOString()
    }

    aliases.push(alias)
  }

  return aliases
}
