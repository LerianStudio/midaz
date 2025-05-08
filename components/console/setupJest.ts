import 'reflect-metadata'

// Polyfill for TextEncoder/TextDecoder which is required by MongoDB connection string URL parsing
if (typeof global.TextEncoder === 'undefined') {
  const { TextEncoder, TextDecoder } = require('util');
  global.TextEncoder = TextEncoder;
  global.TextDecoder = TextDecoder;
}
