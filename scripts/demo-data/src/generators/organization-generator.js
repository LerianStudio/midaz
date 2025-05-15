/**
 * Organization generator for Midaz
 */
import { faker } from '@faker-js/faker/locale/pt_BR';
import fakerUtils from '../utils/faker-utils.js';
import config from '../../config.js';

/**
 * Generates organization data matching API requirements
 * @param {boolean} isCompany - Whether to generate a company (true) or individual (false)
 * @returns {Object} - Organization data
 */
export const generateOrganization = (isCompany = true) => {
  // Setup metadata
  const metadata = {
    createdBy: 'demo-data-generator',
    createdAt: new Date().toISOString(),
    isDemo: true
  };

  // Set up base structure matching the Postman example
  const organization = {
    address: {
      city: faker.location.city(),
      country: "BR",
      line1: faker.location.streetAddress(),
      line2: faker.location.secondaryAddress(),
      state: faker.location.state(),
      zipCode: faker.location.zipCode('#####-###')
    },
    metadata,
    status: {
      code: 'ACTIVE'
    },
    parentOrganizationId: null
  };

  if (isCompany) {
    // Company organization
    const companyName = fakerUtils.generateCompanyName();
    organization.legalName = companyName;
    organization.doingBusinessAs = companyName.split(' ')[0];
    // Must be a string of numbers, no formatting
    organization.legalDocument = fakerUtils.generateDocument(true).replace(/[^0-9]/g, '');
    metadata.type = 'company';
  } else {
    // Individual organization
    const firstName = faker.person.firstName();
    const lastName = faker.person.lastName();
    organization.legalName = `${firstName} ${lastName}`;
    organization.doingBusinessAs = firstName;
    // Must be a string of numbers, no formatting
    organization.legalDocument = fakerUtils.generateDocument(false).replace(/[^0-9]/g, '');
    metadata.type = 'individual';
  }

  return organization;
};

/**
 * Generates multiple organizations
 * @param {number} count - Number of organizations to generate
 * @returns {Array<Object>} - Array of organization data
 */
export const generateOrganizations = (count = 1) => {
  const organizations = [];
  const ratio = config.random.personTypeRatio;
  
  for (let i = 0; i < count; i++) {
    // Determine if this should be a company or individual based on configured ratio
    const isCompany = Math.random() > ratio.individual;
    organizations.push(generateOrganization(isCompany));
  }
  
  return organizations;
};

export default {
  generateOrganization,
  generateOrganizations
};