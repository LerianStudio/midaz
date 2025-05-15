/**
 * Portfolio generator for Midaz
 */
import { faker } from '@faker-js/faker/locale/pt_BR';

// Common portfolio types
const portfolioTypes = [
  { name: 'Conservative', risk: 'low', returnTarget: 'low' },
  { name: 'Moderate', risk: 'medium', returnTarget: 'medium' },
  { name: 'Balanced', risk: 'medium', returnTarget: 'medium' },
  { name: 'Growth', risk: 'high', returnTarget: 'high' },
  { name: 'Aggressive', risk: 'high', returnTarget: 'high' },
  { name: 'Income', risk: 'low', returnTarget: 'medium' },
  { name: 'Value', risk: 'medium', returnTarget: 'medium' },
  { name: 'Tax-Optimized', risk: 'medium', returnTarget: 'medium' }
];

/**
 * Generates portfolio data
 * @param {number} index - Index for type selection
 * @returns {Object} - Portfolio data
 */
export const generatePortfolio = (index = 0) => {
  const portfolioType = portfolioTypes[index % portfolioTypes.length];
  
  return {
    name: portfolioType.name,
    entityId: "00000000-0000-0000-0000-000000000000", // Adding entityId as seen in Postman example
    metadata: {
      createdBy: 'demo-data-generator',
      createdAt: new Date().toISOString(),
      isDemo: true,
      description: `${portfolioType.name} portfolio with ${portfolioType.risk} risk profile`,
      riskProfile: portfolioType.risk,
      returnTarget: portfolioType.returnTarget
    },
    status: {
      code: 'ACTIVE'
    }
  };
};

/**
 * Generates multiple portfolios
 * @param {number} count - Number of portfolios to generate
 * @returns {Array<Object>} - Array of portfolio data
 */
export const generatePortfolios = (count = 1) => {
  const portfolios = [];
  
  for (let i = 0; i < count; i++) {
    portfolios.push(generatePortfolio(i));
  }
  
  return portfolios;
};

export default {
  generatePortfolio,
  generatePortfolios
};