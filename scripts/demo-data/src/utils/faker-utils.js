/**
 * Utilities for data generation using Faker.js
 */
import { fakerPT_BR as faker } from '@faker-js/faker';

/**
 * Generates a valid document (CPF or CNPJ)
 * @param {boolean} isCompany - If true, generates a CNPJ. Otherwise, generates a CPF.
 * @returns {string} - Formatted document
 */
export const generateDocument = (isCompany = false) => {
  if (isCompany) {
    // Generate CNPJ (basic format)
    const n1 = faker.number.int({ min: 10, max: 99 });
    const n2 = faker.number.int({ min: 100, max: 999 });
    const n3 = faker.number.int({ min: 100, max: 999 });
    const n4 = faker.number.int({ min: 1, max: 99 });
    const n5 = faker.number.int({ min: 1, max: 99 });
    
    return `${n1}.${n2}.${n3}/${n4}-${n5}`;
  } else {
    // Generate CPF (basic format)
    const n1 = faker.number.int({ min: 100, max: 999 });
    const n2 = faker.number.int({ min: 100, max: 999 });
    const n3 = faker.number.int({ min: 100, max: 999 });
    const n4 = faker.number.int({ min: 10, max: 99 });
    
    return `${n1}.${n2}.${n3}-${n4}`;
  }
};

/**
 * Generates a complete Brazilian address
 * @returns {Object} - Address data object
 */
export const generateAddress = () => {
  return {
    line1: faker.location.streetAddress(),
    line2: faker.location.secondaryAddress(),
    city: faker.location.city(),
    state: faker.location.state(),
    zipCode: faker.location.zipCode('#####-###'),
    country: 'BR'
  };
};

/**
 * Generates Brazilian phone numbers
 * @param {number} quantity - Number of phone numbers to generate
 * @returns {Array<string>} - Array of phone numbers
 */
export const generatePhoneNumbers = (quantity = 1) => {
  const phones = [];
  for (let i = 0; i < quantity; i++) {
    phones.push(faker.phone.number('+55 ## #####-####'));
  }
  return phones;
};

/**
 * Generates emails
 * @param {number} quantity - Number of emails to generate
 * @returns {Array<string>} - Array of emails
 */
export const generateEmails = (quantity = 1) => {
  const emails = [];
  for (let i = 0; i < quantity; i++) {
    emails.push(faker.internet.email().toLowerCase());
  }
  return emails;
};

/**
 * Generates a Brazilian company name
 * @returns {string} - Company name
 */
export const generateCompanyName = () => {
  const types = ['Ltda.', 'S.A.', 'MEI', 'EIRELI'];
  const type = faker.helpers.arrayElement(types);
  return `${faker.company.name()} ${type}`;
};

/**
 * Generates a random value within a range
 * @param {number} min - Minimum value (inclusive)
 * @param {number} max - Maximum value (inclusive)
 * @returns {number} - Random value
 */
export const randomValue = (min, max) => {
  return faker.number.int({ min, max });
};

/**
 * Generates a random date within a range
 * @param {Date} from - Start date
 * @param {Date} to - End date
 * @returns {Date} - Random date
 */
export const randomDate = (from, to) => {
  return faker.date.between({ from, to });
};

/**
 * Generates a boleto barcode string
 * @returns {string} - Barcode
 */
export const generateBarcodeNumber = () => {
  let barcode = '';
  for (let i = 0; i < 48; i++) {
    barcode += faker.number.int({ min: 0, max: 9 });
  }
  return barcode;
};

/**
 * Randomly selects elements from an array
 * @param {Array} array - Array of elements
 * @param {number} count - Number of elements to select
 * @returns {Array} - Selected elements
 */
export const selectRandom = (array, count = 1) => {
  if (!array || array.length === 0) return [];
  if (count >= array.length) return [...array];
  
  const result = [];
  const copy = [...array];
  
  for (let i = 0; i < count; i++) {
    const index = faker.number.int({ min: 0, max: copy.length - 1 });
    result.push(copy[index]);
    copy.splice(index, 1);
  }
  
  return result;
};

/**
 * Generates transaction data based on type
 * @param {string} type - Transaction type
 * @returns {Object} - Metadata for the transaction
 */
export const generateTransactionMetadata = (type) => {
  switch(type) {
    case 'PIX':
      return {
        type: 'PIX',
        pixType: faker.helpers.arrayElement(['transfer', 'payment', 'withdrawal']),
        pixKey: faker.helpers.arrayElement(['email', 'phone', 'document', 'randomKey']),
        initiator: faker.helpers.arrayElement(['user', 'merchant', 'bank']),
        description: faker.finance.transactionDescription()
      };
      
    case 'TED':
      return {
        type: 'TED',
        bankCode: faker.helpers.arrayElement(['001', '104', '341', '237', '033']),
        accountType: faker.helpers.arrayElement(['checking', 'savings']),
        scheduledDate: faker.date.recent().toISOString().split('T')[0],
        description: faker.finance.transactionDescription()
      };
      
    case 'BOLETO':
      return {
        type: 'BOLETO',
        barcode: generateBarcodeNumber(),
        dueDate: faker.date.soon({ days: 30 }).toISOString().split('T')[0],
        issuer: faker.helpers.arrayElement(['bank', 'utility', 'government', 'retail']),
        description: faker.finance.transactionDescription()
      };
      
    case 'P2P':
      return {
        type: 'P2P',
        platform: faker.helpers.arrayElement(['app', 'website', 'terminal']),
        category: faker.helpers.arrayElement(['purchase', 'subscription', 'donation']),
        merchant: faker.company.name(),
        description: faker.finance.transactionDescription()
      };
      
    default:
      return {
        type: 'GENERIC',
        description: faker.finance.transactionDescription()
      };
  }
};

export default {
  faker,
  generateDocument,
  generateAddress,
  generatePhoneNumbers,
  generateEmails,
  generateCompanyName,
  randomValue,
  randomDate,
  generateBarcodeNumber,
  selectRandom,
  generateTransactionMetadata
};