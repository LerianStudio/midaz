import { faker } from '@faker-js/faker'

/**
 * Test Data Factory using Faker.js
 * Generates realistic, random test data for all Midaz Console domains
 */
export const testDataFactory = {
  /**
   * Organization data generator
   */
  organization: () => ({
    id: faker.string.uuid(),
    name: faker.company.name(),
    legalName: faker.company.name(),
    doingBusinessAs: faker.company.buzzPhrase(),
    legalDocument: faker.string.numeric(14),
    address: {
      line1: faker.location.streetAddress(),
      line2: faker.location.secondaryAddress(),
      city: faker.location.city(),
      state: faker.location.state(),
      country: faker.location.countryCode(),
      zipCode: faker.location.zipCode()
    },
    status: faker.helpers.arrayElement(['active', 'inactive']),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Ledger data generator
   */
  ledger: () => ({
    id: faker.string.uuid(),
    name: `${faker.finance.accountName()}-${faker.string.alphanumeric(6)}`,
    status: faker.helpers.arrayElement(['active', 'inactive']),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Account data generator
   */
  account: () => ({
    id: faker.string.uuid(),
    name: faker.finance.accountName(),
    alias: `@${faker.internet.username()}`,
    assetCode: faker.finance.currencyCode(),
    entityId: faker.string.alphanumeric(10).toUpperCase(),
    type: faker.helpers.arrayElement([
      'deposit',
      'checking',
      'savings',
      'loan',
      'investment',
      'current'
    ]),
    balance: {
      available: faker.number.int({ min: 0, max: 1000000 }),
      onHold: faker.number.int({ min: 0, max: 10000 }),
      scale: faker.number.int({ min: 2, max: 8 })
    },
    allowSending: faker.datatype.boolean(),
    allowReceiving: faker.datatype.boolean(),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Balance data generator
   */
  balance: () => ({
    id: faker.string.uuid(),
    accountId: faker.string.uuid(),
    assetCode: faker.finance.currencyCode(),
    available: faker.number.int({ min: 0, max: 1000000 }),
    onHold: faker.number.int({ min: 0, max: 10000 }),
    scale: faker.number.int({ min: 2, max: 8 }),
    version: faker.number.int({ min: 1, max: 100 }),
    createdAt: faker.date.past().toISOString(),
    updatedAt: faker.date.recent().toISOString()
  }),

  /**
   * Asset data generator
   */
  asset: () => ({
    id: faker.string.uuid(),
    name: faker.finance.currencyName(),
    code: faker.finance.currencyCode(),
    type: faker.helpers.arrayElement([
      'currency',
      'crypto',
      'commodity',
      'others'
    ]),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Transaction data generator
   */
  transaction: () => ({
    id: faker.string.uuid(),
    description: faker.finance.transactionDescription(),
    amount: faker.number.int({ min: 100, max: 100000 }),
    assetCode: faker.finance.currencyCode(),
    status: faker.helpers.arrayElement([
      'pending',
      'approved',
      'rejected',
      'cancelled'
    ]),
    chartOfAccounts: faker.string.alphanumeric(10),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Portfolio data generator
   */
  portfolio: () => ({
    id: faker.string.uuid(),
    name: `Portfolio-${faker.company.buzzNoun()}`,
    entityId: faker.string.uuid(),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Segment data generator
   */
  segment: () => ({
    id: faker.string.uuid(),
    name: `Segment-${faker.company.buzzAdjective()}`,
    metadata: testDataFactory.metadata()
  }),

  /**
   * Account Type data generator
   */
  accountType: () => ({
    id: faker.string.uuid(),
    name: faker.finance.accountName(),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Transaction Route data generator
   */
  transactionRoute: () => ({
    id: faker.string.uuid(),
    name: `Route-${faker.commerce.department()}`,
    description: faker.lorem.sentence(),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Operation Route data generator
   */
  operationRoute: () => ({
    id: faker.string.uuid(),
    name: `OpRoute-${faker.commerce.productName()}`,
    operation: faker.helpers.arrayElement(['credit', 'debit', 'transfer']),
    metadata: testDataFactory.metadata()
  }),

  /**
   * Settings data generator
   */
  settings: () => ({
    id: faker.string.uuid(),
    theme: faker.helpers.arrayElement(['light', 'dark', 'system']),
    language: faker.helpers.arrayElement(['en', 'pt', 'es']),
    timezone: faker.location.timeZone(),
    dateFormat: faker.helpers.arrayElement(['MM/DD/YYYY', 'DD/MM/YYYY', 'YYYY-MM-DD']),
    currency: faker.finance.currencyCode(),
    notifications: {
      email: faker.datatype.boolean(),
      push: faker.datatype.boolean(),
      sms: faker.datatype.boolean()
    }
  }),

  /**
   * User data generator
   */
  user: () => ({
    id: faker.string.uuid(),
    email: faker.internet.email(),
    name: faker.person.fullName(),
    firstName: faker.person.firstName(),
    lastName: faker.person.lastName(),
    phone: faker.phone.number(),
    avatar: faker.image.avatar(),
    role: faker.helpers.arrayElement(['admin', 'user', 'viewer'])
  }),

  /**
   * Metadata generator
   * Generates key-value pairs for metadata fields
   */
  metadata: (count: number = 3): Record<string, string> => {
    const meta: Record<string, string> = {}
    for (let i = 0; i < count; i++) {
      meta[faker.word.noun()] = faker.word.adjective()
    }
    return meta
  },

  /**
   * List generator
   * Generates an array of items using the provided generator function
   */
  list: <T>(generator: () => T, count: number = 10): T[] => {
    return Array.from({ length: count }, () => generator())
  },

  /**
   * Unique name generator with timestamp
   * Useful for ensuring unique entity names in tests
   */
  uniqueName: (prefix: string = 'E2E'): string => {
    return `${prefix}-${faker.string.alphanumeric(8)}-${Date.now()}`
  },

  /**
   * Random enum value picker
   * Useful for selecting random values from predefined enums
   */
  pickEnum: <T>(enumValues: T[]): T => {
    return faker.helpers.arrayElement(enumValues)
  },

  /**
   * Generate partial data (useful for update operations)
   * Takes a full object and returns a subset of its properties
   */
  partial: <T extends Record<string, any>>(
    fullData: T,
    fields: (keyof T)[]
  ): Partial<T> => {
    const partial: Partial<T> = {}
    fields.forEach((field) => {
      partial[field] = fullData[field]
    })
    return partial
  },

  /**
   * Generate invalid data for validation testing
   */
  invalid: {
    email: () => faker.string.alphanumeric(10), // Not a valid email
    phone: () => faker.string.alpha(10), // Not a valid phone
    url: () => faker.string.alphanumeric(10), // Not a valid URL
    uuid: () => faker.string.alphanumeric(10), // Not a valid UUID
    emptyString: () => '',
    tooLongString: (maxLength: number) =>
      faker.string.alphanumeric(maxLength + 100),
    negativeNumber: () => faker.number.int({ min: -1000, max: -1 }),
    futureDate: () => faker.date.future(),
    pastDate: () => faker.date.past()
  },

  /**
   * Generate from OpenAPI schema (placeholder for future enhancement)
   * This can be extended to parse OpenAPI schemas and generate data accordingly
   */
  generateFromSchema: (schema: any): any => {
    // TODO: Implement OpenAPI schema-based generation
    console.warn(
      'generateFromSchema not yet implemented, returning mock data'
    )
    return {}
  }
}

/**
 * Export helper type for test data
 */
export type TestDataFactory = typeof testDataFactory
