/**
 * Centralized configuration for the Midaz demo data generator
 */
export const config = {
  // API configuration
  api: {
    baseUrl: process.env.API_BASE_URL || 'http://localhost',
    onboardingPort: process.env.ONBOARDING_PORT || '3000',
    transactionPort: process.env.TRANSACTION_PORT || '3001',
    timeout: 30000, // Increased timeout for slower responses
    retries: 5,     // Increased retries for better resilience
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${process.env.AUTH_TOKEN || ''}`,
    }
  },

  // Data volumes by size
  volumes: {
    small: {
      organizations: 2,
      ledgersPerOrg: 1,
      assetsPerLedger: 3,
      segmentsPerLedger: 2,
      portfoliosPerLedger: 3,
      accountsPerLedger: 10,
      transactionsPerAccount: 5
    },
    medium: {
      organizations: 5,
      ledgersPerOrg: 2,
      assetsPerLedger: 3,
      segmentsPerLedger: 3,
      portfoliosPerLedger: 5,
      accountsPerLedger: 25,
      transactionsPerAccount: 15
    },
    large: {
      organizations: 10,
      ledgersPerOrg: 2,
      assetsPerLedger: 3,
      segmentsPerLedger: 5,
      portfoliosPerLedger: 8,
      accountsPerLedger: 50,
      transactionsPerAccount: 30
    }
  },

  // Concurrency configuration
  concurrency: {
    organizations: 1,
    ledgers: 2,
    assets: 2,
    segments: 2,
    portfolios: 2,
    accounts: 3,
    transactions: 5
  },

  // Configuration for random data generation
  random: {
    // Ratio of individual vs company (70% individual, 30% company)
    personTypeRatio: {
      individual: 0.7,
      company: 0.3
    },
    // Account types
    accountTypes: ['checking', 'savings', 'investment', 'loan'],
    // Default asset codes
    assetCodes: ['BRL', 'USD', 'GBP'],
    // Transaction values (in cents)
    transactionValue: {
      min: 1000, // R$10.00
      max: 100000 // R$1,000.00
    },
    // Initial balances (in cents)
    initialBalance: {
      min: 100000000, // R$1,000,000.00
      max: 500000000  // R$5,000,000.00
    },
    // Transaction types for metadata
    transactionTypes: [
      { 
        type: 'PIX', 
        metadata: {
          pixType: ['transfer', 'payment', 'withdrawal'],
          pixKey: ['email', 'phone', 'document', 'randomKey']
        }
      },
      { 
        type: 'TED', 
        metadata: {
          bankCode: ['001', '104', '341', '237', '033'],
          accountType: ['checking', 'savings']
        }
      },
      { 
        type: 'BOLETO', 
        metadata: {
          barcode: true,
          dueDate: true,
          issuer: ['bank', 'utility', 'government', 'retail']
        }
      },
      { 
        type: 'P2P', 
        metadata: {
          platform: ['app', 'website', 'terminal'],
          category: ['purchase', 'subscription', 'donation']
        }
      }
    ]
  }
};

export default config;