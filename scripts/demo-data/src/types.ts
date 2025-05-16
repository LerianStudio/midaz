/**
 * Types definitions for Midaz demo data generator
 */

/**
 * Volume size options for data generation
 */
export enum VolumeSize {
  SMALL = 'small',
  MEDIUM = 'medium',
  LARGE = 'large',
}

/**
 * Volume metrics for different sizes
 */
export interface VolumeMetrics {
  organizations: number;
  ledgersPerOrg: number;
  assetsPerLedger: number;
  portfoliosPerLedger: number;
  segmentsPerLedger: number;
  accountsPerLedger: number;
  transactionsPerAccount: number;
}

/**
 * Generator options
 */
export interface GeneratorOptions {
  volume: VolumeSize;
  baseUrl: string;
  onboardingPort: number;
  transactionPort: number;
  concurrency: number;
  debug: boolean;
  authToken?: string;
  seed?: number;
}

/**
 * Entity generator interface
 */
export interface EntityGenerator<T> {
  generate(count: number, parentId?: string): Promise<T[]>;
  generateOne(parentId?: string): Promise<T>;
  exists(id: string): Promise<boolean>;
}

/**
 * Generator state management
 */
export interface GeneratorState {
  organizationIds: string[];
  ledgerIds: Map<string, string[]>; // orgId -> ledgerIds
  assetIds: Map<string, string[]>; // ledgerId -> assetIds
  assetCodes: Map<string, string[]>; // ledgerId -> assetCodes
  portfolioIds: Map<string, string[]>; // ledgerId -> portfolioIds
  segmentIds: Map<string, string[]>; // ledgerId -> segmentIds
  accountIds: Map<string, string[]>; // ledgerId -> accountIds
  accountAliases: Map<string, string[]>; // ledgerId -> accountAliases
  transactionIds: Map<string, string[]>; // ledgerId -> transactionIds
  accountAssets: Map<string, Map<string, string>>; // ledgerId -> (accountId -> assetCode)
}

/**
 * Generation metrics
 */
export interface GenerationMetrics {
  startTime: Date;
  endTime?: Date;
  totalOrganizations: number;
  totalLedgers: number;
  totalAssets: number;
  totalPortfolios: number;
  totalSegments: number;
  totalAccounts: number;
  totalTransactions: number;
  errors: number;
  retries: number;
  duration(): number;
}

/**
 * Person type - Individual (PF) or Company (PJ)
 */
export enum PersonType {
  INDIVIDUAL = 'PF',
  COMPANY = 'PJ',
}

/**
 * Person data structure for organization creation
 */
export interface PersonData {
  type: PersonType;
  name: string;
  document: string; // CPF or CNPJ
  tradingName?: string;
  address: {
    line1: string;
    line2?: string;
    city: string;
    state: string;
    zipCode: string;
    country: string;
  };
}
