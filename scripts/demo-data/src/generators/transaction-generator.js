/**
 * Transaction generator for Midaz
 */
import { faker } from '@faker-js/faker/locale/pt_BR';
import fakerUtils from '../utils/faker-utils.js';
import config from '../../config.js';

/**
 * Generates an initial deposit transaction
 * @param {string} accountAlias - Destination account alias
 * @param {string} assetCode - Asset code (e.g., BRL, USD)
 * @param {number} value - Transaction value in cents
 * @returns {Object} - Transaction data
 */
export const generateInitialDeposit = (accountAlias, assetCode, value) => {
  return {
    code: `INIT_${faker.string.alphanumeric(8).toUpperCase()}`,
    description: `Initial deposit for ${accountAlias}`,
    chartOfAccountsGroupName: 'DEPOSIT',
    metadata: {
      createdBy: 'demo-data-generator',
      createdAt: new Date().toISOString(),
      isDemo: true,
      transactionType: 'INITIAL_DEPOSIT'
    },
    send: {
      asset: assetCode,
      value,
      scale: 2, // Adding scale back for transactions - required by the API
      source: {
        from: [
          {
            account: `@external/${assetCode}`,
            amount: {
              asset: assetCode,
              value,
              scale: 2 // Adding scale back for amount
            },
            chartOfAccounts: 'DEPOSIT',
            description: `Initial deposit from external source`,
            metadata: {
              source: 'external',
              type: 'deposit'
            }
          }
        ]
      },
      distribute: {
        to: [
          {
            account: accountAlias,
            amount: {
              asset: assetCode,
              value,
              scale: 2 // Adding scale back for amount
            },
            chartOfAccounts: 'DEPOSIT',
            description: `Initial deposit to account`,
            metadata: {
              destination: 'account',
              type: 'deposit'
            }
          }
        ]
      }
    }
  };
};

/**
 * Generates a transaction between accounts
 * @param {string} fromAccountAlias - Source account alias
 * @param {string} toAccountAlias - Destination account alias
 * @param {string} assetCode - Asset code (e.g., BRL, USD)
 * @param {number} value - Transaction value in cents
 * @param {string} type - Transaction type (PIX, TED, etc.)
 * @returns {Object} - Transaction data
 */
export const generateAccountTransaction = (fromAccountAlias, toAccountAlias, assetCode, value, type) => {
  // Generate metadata based on transaction type
  const metadata = {
    ...fakerUtils.generateTransactionMetadata(type),
    createdBy: 'demo-data-generator',
    createdAt: new Date().toISOString(),
    isDemo: true
  };
  
  // Generate transaction code
  const code = `${type}_${faker.string.alphanumeric(8).toUpperCase()}`;
  
  // Generate transaction description
  const description = metadata.description || `${type} transaction from ${fromAccountAlias} to ${toAccountAlias}`;
  
  return {
    code,
    description,
    chartOfAccountsGroupName: type,
    metadata,
    send: {
      asset: assetCode,
      value,
      scale: 2, // Adding scale back for transactions - required by the API
      source: {
        from: [
          {
            account: fromAccountAlias,
            amount: {
              asset: assetCode,
              value,
              scale: 2 // Adding scale back for amount
            },
            chartOfAccounts: type,
            description: `${type} outbound transfer`,
            metadata: {
              source: 'account',
              type: 'debit',
              transactionType: type
            }
          }
        ]
      },
      distribute: {
        to: [
          {
            account: toAccountAlias,
            amount: {
              asset: assetCode,
              value,
              scale: 2 // Adding scale back for amount
            },
            chartOfAccounts: type,
            description: `${type} inbound transfer`,
            metadata: {
              destination: 'account',
              type: 'credit',
              transactionType: type
            }
          }
        ]
      }
    }
  };
};

/**
 * Generates a random transaction value
 * @param {number} min - Minimum value in cents
 * @param {number} max - Maximum value in cents
 * @returns {number} - Transaction value in cents
 */
export const generateTransactionValue = (min = config.random.transactionValue.min, max = config.random.transactionValue.max) => {
  return fakerUtils.randomValue(min, max);
};

/**
 * Generates initial deposit transactions for a list of accounts
 * @param {Array<Object>} accounts - List of account objects with their aliases
 * @param {string} assetCode - Asset code (e.g., BRL, USD)
 * @returns {Array<Object>} - Array of initial deposit transactions
 */
export const generateInitialDeposits = (accounts, assetCode = 'BRL') => {
  return accounts.map(account => {
    const value = fakerUtils.randomValue(
      config.random.initialBalance.min,
      config.random.initialBalance.max
    );
    return generateInitialDeposit(account.alias, assetCode, value);
  });
};

/**
 * Generates multiple transactions between accounts
 * @param {Array<Object>} accounts - List of account objects with their aliases
 * @param {string} assetCode - Asset code (e.g., BRL, USD)
 * @param {number} count - Number of transactions to generate per account
 * @returns {Array<Object>} - Array of transactions
 */
export const generateTransactions = (accounts, assetCode = 'BRL', count = 5) => {
  if (accounts.length < 2) {
    throw new Error('Need at least 2 accounts to generate transactions between accounts');
  }
  
  const transactions = [];
  const transactionTypes = ['PIX', 'TED', 'BOLETO', 'P2P'];
  
  // For each account, generate transactions to other accounts
  accounts.forEach(fromAccount => {
    for (let i = 0; i < count; i++) {
      // Select a random destination account that is not the source account
      let toAccount;
      do {
        toAccount = faker.helpers.arrayElement(accounts);
      } while (toAccount.alias === fromAccount.alias);
      
      // Generate a random transaction value
      const value = generateTransactionValue();
      
      // Select a random transaction type
      const type = faker.helpers.arrayElement(transactionTypes);
      
      // Generate the transaction
      transactions.push(
        generateAccountTransaction(
          fromAccount.alias,
          toAccount.alias,
          assetCode,
          value,
          type
        )
      );
    }
  });
  
  return transactions;
};

export default {
  generateInitialDeposit,
  generateAccountTransaction,
  generateTransactionValue,
  generateInitialDeposits,
  generateTransactions
};