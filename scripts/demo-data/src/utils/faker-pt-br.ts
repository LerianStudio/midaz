/**
 * Brazilian locale helpers for Faker.js
 */



// Handle Faker import consistently
// eslint-disable-next-line @typescript-eslint/no-var-requires
const faker = require('faker');

// Initialize faker with pt_BR locale
// This works across different versions of faker
require('faker/locale/pt_BR');
import { PersonType, PersonData } from '../types';
import { PERSON_TYPE_DISTRIBUTION } from '../config';

// Use Brazilian Portuguese locale
// Note: We're importing the base faker and will use locale-specific methods

/**
 * Generate a valid CPF number (for individuals)
 * Algorithm based on official rules
 */
export function generateCPF(): string {
  const generateDigit = (digits: number[]): number => {
    let sum = 0;
    for (let i = 0; i < digits.length; i++) {
      sum += digits[i] * (digits.length + 1 - i);
    }
    const remainder = sum % 11;
    return remainder < 2 ? 0 : 11 - remainder;
  };

  // Generate first 9 random digits
  const digits = Array.from({ length: 9 }, () => Math.floor(Math.random() * 10));

  // Calculate first verification digit
  const digit1 = generateDigit(digits);
  digits.push(digit1);

  // Calculate second verification digit
  const digit2 = generateDigit(digits);
  digits.push(digit2);

  // Format as CPF: XXX.XXX.XXX-XX
  return `${digits.slice(0, 3).join('')}.${digits.slice(3, 6).join('')}.${digits
    .slice(6, 9)
    .join('')}-${digits.slice(9).join('')}`;
}

/**
 * Generate a valid CNPJ number (for companies)
 * Algorithm based on official rules
 */
export function generateCNPJ(): string {
  const generateDigit = (digits: number[], weights: number[]): number => {
    let sum = 0;
    for (let i = 0; i < digits.length; i++) {
      sum += digits[i] * weights[i];
    }
    const remainder = sum % 11;
    return remainder < 2 ? 0 : 11 - remainder;
  };

  // Generate first 12 random digits (8 base + 4 branch)
  const digits = Array.from({ length: 12 }, () => Math.floor(Math.random() * 10));

  // Calculate first verification digit
  const weights1 = [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
  const digit1 = generateDigit(digits, weights1);
  digits.push(digit1);

  // Calculate second verification digit
  const weights2 = [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
  const digit2 = generateDigit(digits, weights2);
  digits.push(digit2);

  // Format as CNPJ: XX.XXX.XXX/YYYY-ZZ
  return `${digits.slice(0, 2).join('')}.${digits.slice(2, 5).join('')}.${digits
    .slice(5, 8)
    .join('')}/${digits.slice(8, 12).join('')}-${digits.slice(12).join('')}`;
}

/**
 * Generate random person data (individual or company)
 * Respects the distribution of 70% individuals (PF) and 30% companies (PJ)
 */
export function generatePersonData(): PersonData {
  // Determine if this is an individual or company based on distribution
  const isIndividual = Math.random() * 100 < PERSON_TYPE_DISTRIBUTION.individualPercentage;

  if (isIndividual) {
    // Generate data for individual (PF)
    const firstName = faker.name.firstName();
    const lastName = faker.name.lastName();
    const fullName = `${firstName} ${lastName}`;

    return {
      type: PersonType.INDIVIDUAL,
      name: fullName,
      document: generateCPF(),
      address: {
        line1: faker.address.streetAddress(),
        line2: faker.address.secondaryAddress(),
        city: faker.address.city(),
        state: faker.address.stateAbbr(),
        zipCode: faker.address.zipCode(),
        country: 'BR',
      },
    };
  } else {
    // Generate data for company (PJ)
    const companyName = faker.company.companyName();
    const tradingName = faker.company.companySuffix();

    return {
      type: PersonType.COMPANY,
      name: companyName,
      document: generateCNPJ(),
      tradingName: `${companyName} ${tradingName}`,
      address: {
        line1: faker.address.streetAddress(),
        line2: faker.address.secondaryAddress(),
        city: faker.address.city(),
        state: faker.address.stateAbbr(),
        zipCode: faker.address.zipCode(),
        country: 'BR',
      },
    };
  }
}

/**
 * Generate a random amount in BRL cents within range
 */
export function generateAmount(
  min = 10,
  max = 10000,
  scale = 2
): {
  value: number;
  formatted: string;
} {
  // Generate a random amount within range
  const scaleFactor = Math.pow(10, scale);
  const minValue = min * scaleFactor;
  const maxValue = max * scaleFactor;
  const value = Math.floor(Math.random() * (maxValue - minValue + 1)) + minValue;

  // Format as BRL
  const formatted = new Intl.NumberFormat('pt-BR', {
    style: 'currency',
    currency: 'BRL',
    minimumFractionDigits: scale,
    maximumFractionDigits: scale,
  }).format(value / scaleFactor);

  return {
    value,
    formatted,
  };
}

/**
 * Generate a unique alias for an account
 */
export function generateAccountAlias(type: string, index: number): string {
  const prefix = type.slice(0, 3).toUpperCase();
  const randomSuffix = Math.floor(Math.random() * 10000)
    .toString()
    .padStart(4, '0');
  return `${prefix}_${randomSuffix}_${index}`;
}
