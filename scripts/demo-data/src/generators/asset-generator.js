/**
 * Asset generator for Midaz
 */
import config from '../../config.js';

/**
 * Generates asset data
 * @param {string} code - Asset code (e.g., USD, BRL)
 * @returns {Object} - Asset data
 */
export const generateAsset = (code) => {
  // Asset data - including type field as required by API
  const asset = {
    code: code,
    name: getAssetName(code),
    type: "currency", // Adding the required type field
    metadata: {
      createdBy: 'demo-data-generator',
      createdAt: new Date().toISOString(),
      isDemo: true
    },
    status: {
      code: 'ACTIVE'
    }
  };

  return asset;
};

/**
 * Generates multiple assets
 * @param {Array<string>} codes - Asset codes to generate
 * @returns {Array<Object>} - Array of asset data
 */
export const generateAssets = (codes = config.random.assetCodes) => {
  return codes.map(code => generateAsset(code));
};

/**
 * Gets the full name for an asset code
 * @param {string} code - Asset code
 * @returns {string} - Asset full name
 */
function getAssetName(code) {
  const assetNames = {
    'BRL': 'Brazilian Real',
    'USD': 'United States Dollar',
    'EUR': 'Euro',
    'GBP': 'Great Britain Pound',
    'JPY': 'Japanese Yen',
    'CAD': 'Canadian Dollar',
    'AUD': 'Australian Dollar',
    'CHF': 'Swiss Franc',
    'CNY': 'Chinese Yuan',
    'HKD': 'Hong Kong Dollar',
    'MXN': 'Mexican Peso'
  };

  return assetNames[code] || `${code} Asset`;
}

export default {
  generateAsset,
  generateAssets
};