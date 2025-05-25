/**
 * Tests for transaction helper functions
 */

import {
  chunkArray,
  wait,
  groupAccountsByAsset,
  calculateOptimalConcurrency,
  formatErrorMessage,
  extractUniqueErrorMessages,
} from '../../../../src/generators/transactions/helpers/transaction-helpers';
import { AccountWithAsset } from '../../../../src/generators/transactions/strategies/transaction-strategies';

describe('Transaction Helpers', () => {
  describe('chunkArray', () => {
    it('should chunk array into specified sizes', () => {
      const array = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
      const chunks = chunkArray(array, 3);
      
      expect(chunks).toHaveLength(4);
      expect(chunks[0]).toEqual([1, 2, 3]);
      expect(chunks[1]).toEqual([4, 5, 6]);
      expect(chunks[2]).toEqual([7, 8, 9]);
      expect(chunks[3]).toEqual([10]);
    });

    it('should handle empty array', () => {
      const chunks = chunkArray([], 3);
      expect(chunks).toHaveLength(0);
    });

    it('should handle chunk size larger than array', () => {
      const array = [1, 2, 3];
      const chunks = chunkArray(array, 10);
      
      expect(chunks).toHaveLength(1);
      expect(chunks[0]).toEqual([1, 2, 3]);
    });
  });

  describe('wait', () => {
    it('should wait for specified duration', async () => {
      const start = Date.now();
      await wait(100);
      const elapsed = Date.now() - start;
      
      expect(elapsed).toBeGreaterThanOrEqual(100);
      expect(elapsed).toBeLessThan(150); // Allow some margin
    });
  });

  describe('groupAccountsByAsset', () => {
    it('should group accounts by asset code', () => {
      const accounts: AccountWithAsset[] = [
        { accountId: 'acc1', accountAlias: 'alias1', assetCode: 'BRL' },
        { accountId: 'acc2', accountAlias: 'alias2', assetCode: 'USD' },
        { accountId: 'acc3', accountAlias: 'alias3', assetCode: 'BRL' },
        { accountId: 'acc4', accountAlias: 'alias4', assetCode: 'USD' },
        { accountId: 'acc5', accountAlias: 'alias5', assetCode: 'EUR' },
      ];

      const grouped = groupAccountsByAsset(accounts);

      expect(grouped.size).toBe(3);
      expect(grouped.get('BRL')).toHaveLength(2);
      expect(grouped.get('USD')).toHaveLength(2);
      expect(grouped.get('EUR')).toHaveLength(1);
    });

    it('should handle empty array', () => {
      const grouped = groupAccountsByAsset([]);
      expect(grouped.size).toBe(0);
    });
  });

  describe('calculateOptimalConcurrency', () => {
    it('should calculate concurrency based on asset type count', () => {
      const concurrency = calculateOptimalConcurrency(100, 5, 50);
      expect(concurrency).toBe(10); // min(50/5, 10, 100) = 10
    });

    it('should not exceed item count', () => {
      const concurrency = calculateOptimalConcurrency(5, 2, 50);
      expect(concurrency).toBe(5); // min(50/2, 10, 5) = 5
    });

    it('should have minimum concurrency of 2', () => {
      const concurrency = calculateOptimalConcurrency(100, 30, 50);
      expect(concurrency).toBe(2); // max(2, floor(50/30)) = 2
    });

    it('should respect max concurrency of 10', () => {
      const concurrency = calculateOptimalConcurrency(100, 1, 50);
      expect(concurrency).toBe(10); // min(50/1, 10, 100) = 10
    });
  });

  describe('formatErrorMessage', () => {
    it('should format Error instance', () => {
      const error = new Error('Test error');
      expect(formatErrorMessage(error)).toBe('Test error');
    });

    it('should format object with message property', () => {
      const error = { message: 'Object error' };
      expect(formatErrorMessage(error)).toBe('Object error');
    });

    it('should stringify object without message property', () => {
      const error = { code: 'ERR001', detail: 'Some detail' };
      expect(formatErrorMessage(error)).toBe(JSON.stringify(error));
    });

    it('should convert primitive values to string', () => {
      expect(formatErrorMessage('String error')).toBe('String error');
      expect(formatErrorMessage(123)).toBe('123');
      expect(formatErrorMessage(true)).toBe('true');
    });

    it('should handle null and undefined', () => {
      expect(formatErrorMessage(null)).toBe('Unknown error');
      expect(formatErrorMessage(undefined)).toBe('Unknown error');
    });
  });

  describe('extractUniqueErrorMessages', () => {
    it('should extract unique error messages from failed results', () => {
      const results = [
        { status: 'success' },
        { status: 'failed', error: { message: 'Error 1' } },
        { status: 'failed', error: { message: 'Error 2' } },
        { status: 'failed', error: { message: 'Error 1' } }, // Duplicate
        { status: 'success' },
        { status: 'failed', error: { message: 'Error 3' } },
      ];

      const uniqueErrors = extractUniqueErrorMessages(results);

      expect(uniqueErrors.size).toBe(3);
      expect(uniqueErrors.has('Error 1')).toBe(true);
      expect(uniqueErrors.has('Error 2')).toBe(true);
      expect(uniqueErrors.has('Error 3')).toBe(true);
    });

    it('should handle results without error messages', () => {
      const results = [
        { status: 'failed' },
        { status: 'failed', error: {} },
        { status: 'failed', error: { message: 'Valid error' } },
      ];

      const uniqueErrors = extractUniqueErrorMessages(results);

      expect(uniqueErrors.size).toBe(2);
      expect(uniqueErrors.has('Unknown error')).toBe(true);
      expect(uniqueErrors.has('Valid error')).toBe(true);
    });

    it('should return empty set for no failures', () => {
      const results = [
        { status: 'success' },
        { status: 'success' },
      ];

      const uniqueErrors = extractUniqueErrorMessages(results);

      expect(uniqueErrors.size).toBe(0);
    });
  });
});