/**
 * Ledger generator for Midaz
 */
import { faker } from '@faker-js/faker/locale/pt_BR';

/**
 * Generates ledger data
 * @param {Object} organization - Parent organization data
 * @param {number} index - Index for naming uniqueness
 * @returns {Object} - Ledger data
 */
export const generateLedger = (organization, index = 0) => {
  const orgName = organization.doingBusinessAs || organization.legalName.split(' ')[0];
  const ledgerTypes = ['main', 'secondary', 'test', 'development', 'reporting'];
  const ledgerType = ledgerTypes[index % ledgerTypes.length];
  
  return {
    name: `${orgName} ${ledgerType} ledger`.trim(),
    metadata: {
      createdBy: 'demo-data-generator',
      createdAt: new Date().toISOString(),
      isDemo: true,
      index,
      type: ledgerType
    },
    status: {
      code: 'ACTIVE'
    }
  };
};

/**
 * Generates multiple ledgers for an organization
 * @param {Object} organization - Parent organization data
 * @param {number} count - Number of ledgers to generate
 * @returns {Array<Object>} - Array of ledger data
 */
export const generateLedgers = (organization, count = 1) => {
  const ledgers = [];
  
  for (let i = 0; i < count; i++) {
    ledgers.push(generateLedger(organization, i));
  }
  
  return ledgers;
};

export default {
  generateLedger,
  generateLedgers
};