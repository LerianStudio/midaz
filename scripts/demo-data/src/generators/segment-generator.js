/**
 * Segment generator for Midaz
 */
import { faker } from '@faker-js/faker/locale/pt_BR';

// Common segment types
const segmentTypes = [
  { name: 'Retail' },
  { name: 'Corporate' },
  { name: 'Private Banking' },
  { name: 'Small Business' },
  { name: 'Mortgage' },
  { name: 'Credit Card' },
  { name: 'Investment' },
  { name: 'Transaction' }
];

/**
 * Generates segment data
 * @param {number} index - Index for type selection
 * @returns {Object} - Segment data
 */
export const generateSegment = (index = 0) => {
  const segmentType = segmentTypes[index % segmentTypes.length];
  
  return {
    name: segmentType.name,
    // Removed code field as it's not supported by the API
    metadata: {
      createdBy: 'demo-data-generator',
      createdAt: new Date().toISOString(),
      isDemo: true,
      description: `${segmentType.name} segment for customer classification`
    },
    status: {
      code: 'ACTIVE'
    }
  };
};

/**
 * Generates multiple segments
 * @param {number} count - Number of segments to generate
 * @returns {Array<Object>} - Array of segment data
 */
export const generateSegments = (count = 1) => {
  const segments = [];
  
  for (let i = 0; i < count; i++) {
    segments.push(generateSegment(i));
  }
  
  return segments;
};

export default {
  generateSegment,
  generateSegments
};