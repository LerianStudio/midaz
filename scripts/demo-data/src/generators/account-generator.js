/**
 * Account generator for Midaz
 */
import { faker } from '@faker-js/faker/locale/pt_BR';
import fakerUtils from '../utils/faker-utils.js';
import config from '../../config.js';

// Account types and their properties
const accountTypeProps = {
  checking: { 
    namePrefixes: ['Checking', 'Current', 'Main', 'Primary'],
    namePostfixes: ['Account', 'Fund', ''],
    type: 'deposit'
  },
  savings: { 
    namePrefixes: ['Savings', 'Reserve', 'Emergency'],
    namePostfixes: ['Account', 'Fund', 'Savings'],
    type: 'deposit'
  },
  investment: { 
    namePrefixes: ['Investment', 'Trading', 'Market'],
    namePostfixes: ['Account', 'Portfolio', 'Fund'],
    type: 'deposit'
  },
  loan: { 
    namePrefixes: ['Loan', 'Credit', 'Financing'],
    namePostfixes: ['Account', 'Facility', 'Line'],
    type: 'loan'
  }
};

/**
 * Generates a unique account alias
 * @param {string} type - Account type
 * @param {number} index - Index for uniqueness
 * @returns {string} - Account alias
 */
const generateAccountAlias = (type, index) => {
  const prefix = faker.helpers.arrayElement(accountTypeProps[type].namePrefixes);
  const postfix = faker.helpers.arrayElement(accountTypeProps[type].namePostfixes);
  const randomNum = String(index).padStart(4, '0');
  
  return `@${prefix.toLowerCase()}_${randomNum}`.replace(/\s+/g, '_');
};

/**
 * Generates account data
 * @param {Array<Object>} segments - Available segments to assign to the account
 * @param {Array<Object>} portfolios - Available portfolios to assign to the account
 * @param {string} accountType - Account type (checking, savings, etc.)
 * @param {number} index - Index for uniqueness
 * @returns {Object} - Account data
 */
export const generateAccount = (segments = [], portfolios = [], accountType = 'checking', index = 0) => {
  // Get props for this account type
  const props = accountTypeProps[accountType] || accountTypeProps.checking;
  
  // Create name and alias
  const prefix = faker.helpers.arrayElement(props.namePrefixes);
  const postfix = faker.helpers.arrayElement(props.namePostfixes);
  const name = `${prefix} ${postfix}`.trim();
  const alias = generateAccountAlias(accountType, index);
  
  // Generate account data
  const account = {
    name,
    alias,
    type: props.type,  // Set type based on account type mapping
    assetCode: "BRL",  // Default asset code
    entityId: `ACC-${faker.string.alphanumeric(8).toUpperCase()}`, // Required field
    parentAccountId: null,  // Required field
    portfolioId: null,  // Required field
    segmentId: null,  // Required field
    metadata: {
      createdBy: 'demo-data-generator',
      createdAt: new Date().toISOString(),
      isDemo: true,
      accountIndex: index
    },
    status: {
      code: 'ACTIVE'
    }
  };
  
  // Add segment if available
  if (segments.length > 0) {
    const segment = faker.helpers.arrayElement(segments);
    account.segmentId = segment.id;
  }
  
  // Add portfolio if available
  if (portfolios.length > 0) {
    const portfolio = faker.helpers.arrayElement(portfolios);
    account.portfolioId = portfolio.id;
  }
  
  // Add additional metadata based on account type
  switch(accountType) {
    case 'checking':
      account.metadata.overdraftLimit = fakerUtils.randomValue(100000, 500000); // $1,000 - $5,000
      break;
    case 'savings':
      account.metadata.interestRate = (Math.random() * 4 + 1).toFixed(2); // 1% - 5%
      break;
    case 'investment':
      account.metadata.riskProfile = faker.helpers.arrayElement(['low', 'medium', 'high']);
      account.metadata.strategy = faker.helpers.arrayElement(['conservative', 'balanced', 'growth']);
      break;
    case 'loan':
      account.metadata.interestRate = (Math.random() * 10 + 5).toFixed(2); // 5% - 15%
      account.metadata.termMonths = faker.helpers.arrayElement([12, 24, 36, 48, 60]);
      break;
  }
  
  return account;
};

/**
 * Generates multiple accounts
 * @param {Array<Object>} segments - Available segments to assign to accounts
 * @param {Array<Object>} portfolios - Available portfolios to assign to accounts
 * @param {number} count - Number of accounts to generate
 * @returns {Array<Object>} - Array of account data
 */
export const generateAccounts = (segments = [], portfolios = [], count = 1) => {
  const accounts = [];
  const accountTypes = Object.keys(accountTypeProps);
  
  for (let i = 0; i < count; i++) {
    // Distribute account types: 60% checking, 20% savings, 10% investment, 10% loan
    let accountType;
    const rand = Math.random();
    if (rand < 0.6) {
      accountType = 'checking';
    } else if (rand < 0.8) {
      accountType = 'savings';
    } else if (rand < 0.9) {
      accountType = 'investment';
    } else {
      accountType = 'loan';
    }
    
    // Generate the account
    accounts.push(generateAccount(segments, portfolios, accountType, i));
  }
  
  return accounts;
};

export default {
  generateAccount,
  generateAccounts
};