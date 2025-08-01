import 'reflect-metadata'

// Polyfill for TextEncoder/TextDecoder which is required by MongoDB connection string URL parsing
if (typeof global.TextEncoder === 'undefined') {
  const { TextEncoder, TextDecoder } = require('util')
  global.TextEncoder = TextEncoder
  global.TextDecoder = TextDecoder
}

// Mock ESM-only packages that cause Jest parsing issues
jest.mock('openid-client', () => ({}))
jest.mock('jose', () => ({}))
jest.mock('next-auth', () => ({
  default: {},
  __esModule: true
}))
jest.mock('next-auth/react', () => ({
  signOut: jest.fn(),
  __esModule: true
}))

jest.mock('mongoose', () => ({}))
jest.mock('mongodb', () => ({}))
jest.mock('bson', () => ({}))
