/**
 * Tests for circuit breaker implementation
 */

import { CircuitBreaker, CircuitState, createCircuitBreaker } from '../../../src/utils/circuit-breaker';

// Helper function to wait for a specified time
const wait = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

describe('CircuitBreaker', () => {
  let circuitBreaker: CircuitBreaker;

  beforeEach(() => {
    circuitBreaker = createCircuitBreaker({
      failureThreshold: 3,
      recoveryTimeout: 100, // Short timeout for testing
      monitoringPeriod: 1000,
      minimumRequests: 2,
      successThreshold: 0.5,
    });
  });

  describe('initial state', () => {
    it('should start in CLOSED state', () => {
      const stats = circuitBreaker.getStats();
      expect(stats.state).toBe(CircuitState.CLOSED);
      expect(stats.failures).toBe(0);
      expect(stats.successes).toBe(0);
      expect(stats.requests).toBe(0);
    });

    it('should be available initially', () => {
      expect(circuitBreaker.isAvailable()).toBe(true);
    });
  });

  describe('successful executions', () => {
    it('should execute successful operations', async () => {
      const mockOperation = jest.fn().mockResolvedValue('success');

      const result = await circuitBreaker.execute(mockOperation);

      expect(result).toBe('success');
      expect(mockOperation).toHaveBeenCalledTimes(1);

      const stats = circuitBreaker.getStats();
      expect(stats.state).toBe(CircuitState.CLOSED);
      expect(stats.successes).toBe(1);
      expect(stats.failures).toBe(0);
      expect(stats.requests).toBe(1);
    });

    it('should track multiple successful executions', async () => {
      const mockOperation = jest.fn().mockResolvedValue('success');

      await circuitBreaker.execute(mockOperation);
      await circuitBreaker.execute(mockOperation);
      await circuitBreaker.execute(mockOperation);

      const stats = circuitBreaker.getStats();
      expect(stats.successes).toBe(3);
      expect(stats.failures).toBe(0);
      expect(stats.requests).toBe(3);
    });
  });

  describe('failed executions', () => {
    it('should track failed executions', async () => {
      const mockOperation = jest.fn().mockRejectedValue(new Error('test error'));

      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow('test error');

      const stats = circuitBreaker.getStats();
      expect(stats.failures).toBe(1);
      expect(stats.successes).toBe(0);
      expect(stats.requests).toBe(1);
      expect(stats.lastFailureTime).toBeDefined();
    });

    it('should remain closed with failures below threshold', async () => {
      const mockOperation = jest.fn().mockRejectedValue(new Error('test error'));

      // Execute 2 failures (below threshold of 3)
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();

      const stats = circuitBreaker.getStats();
      expect(stats.state).toBe(CircuitState.CLOSED);
      expect(stats.failures).toBe(2);
      expect(circuitBreaker.isAvailable()).toBe(true);
    });

    it('should open circuit when failure threshold is reached', async () => {
      const mockOperation = jest.fn().mockRejectedValue(new Error('test error'));

      // Execute failures to reach threshold
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();

      const stats = circuitBreaker.getStats();
      expect(stats.state).toBe(CircuitState.OPEN);
      expect(stats.failures).toBe(3);
    });
  });

  describe('circuit states', () => {
    it('should reject requests when circuit is OPEN', async () => {
      const mockOperation = jest.fn().mockRejectedValue(new Error('test error'));

      // Open the circuit
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();

      // Circuit should be open now
      const stats = circuitBreaker.getStats();
      expect(stats.state).toBe(CircuitState.OPEN);

      // Next execution should be rejected without calling the operation
      const newMockOperation = jest.fn().mockResolvedValue('success');
      await expect(circuitBreaker.execute(newMockOperation)).rejects.toThrow('Circuit breaker is OPEN');
      expect(newMockOperation).not.toHaveBeenCalled();
    });

    it('should transition to HALF_OPEN after recovery timeout', async () => {
      const mockOperation = jest.fn().mockRejectedValue(new Error('test error'));

      // Open the circuit
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();

      expect(circuitBreaker.getStats().state).toBe(CircuitState.OPEN);

      // Wait for recovery timeout
      await wait(150);

      // Next execution should transition to HALF_OPEN
      const successOperation = jest.fn().mockResolvedValue('success');
      const result = await circuitBreaker.execute(successOperation);

      expect(result).toBe('success');
      expect(successOperation).toHaveBeenCalledTimes(1);
    });

    it('should close circuit from HALF_OPEN on successful execution', async () => {
      const mockFailOperation = jest.fn().mockRejectedValue(new Error('test error'));

      // Open the circuit
      await expect(circuitBreaker.execute(mockFailOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockFailOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockFailOperation)).rejects.toThrow();

      // Wait for recovery timeout
      await wait(150);

      // Execute successful operation to close circuit
      const successOperation = jest.fn().mockResolvedValue('success');
      await circuitBreaker.execute(successOperation);

      const stats = circuitBreaker.getStats();
      expect(stats.state).toBe(CircuitState.CLOSED);
    });

    it('should reopen circuit from HALF_OPEN on failure', async () => {
      const mockFailOperation = jest.fn().mockRejectedValue(new Error('test error'));

      // Open the circuit
      await expect(circuitBreaker.execute(mockFailOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockFailOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockFailOperation)).rejects.toThrow();

      // Wait for recovery timeout
      await wait(150);

      // Fail again in HALF_OPEN state
      await expect(circuitBreaker.execute(mockFailOperation)).rejects.toThrow();

      const stats = circuitBreaker.getStats();
      expect(stats.state).toBe(CircuitState.OPEN);
    });
  });

  describe('manual operations', () => {
    it('should manually reset circuit', async () => {
      const mockOperation = jest.fn().mockRejectedValue(new Error('test error'));

      // Open the circuit
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();
      await expect(circuitBreaker.execute(mockOperation)).rejects.toThrow();

      expect(circuitBreaker.getStats().state).toBe(CircuitState.OPEN);

      // Manually reset
      circuitBreaker.manualReset();

      const stats = circuitBreaker.getStats();
      expect(stats.state).toBe(CircuitState.CLOSED);
      expect(stats.failures).toBe(0);
      expect(stats.successes).toBe(0);
      expect(stats.requests).toBe(0);
    });
  });

  describe('createCircuitBreaker factory', () => {
    it('should create circuit breaker with default options', () => {
      const cb = createCircuitBreaker({});
      const stats = cb.getStats();

      expect(stats.state).toBe(CircuitState.CLOSED);
      expect(cb.isAvailable()).toBe(true);
    });

    it('should create circuit breaker with custom options', () => {
      const cb = createCircuitBreaker({
        failureThreshold: 10,
        recoveryTimeout: 30000,
      });

      expect(cb.isAvailable()).toBe(true);
    });
  });

  describe('edge cases', () => {
    it('should handle operations that throw non-Error objects', async () => {
      const mockOperation = jest.fn().mockRejectedValue('string error');

      await expect(circuitBreaker.execute(mockOperation)).rejects.toBe('string error');

      const stats = circuitBreaker.getStats();
      expect(stats.failures).toBe(1);
    });

    it('should handle operations that return undefined', async () => {
      const mockOperation = jest.fn().mockResolvedValue(undefined);

      const result = await circuitBreaker.execute(mockOperation);

      expect(result).toBeUndefined();
      expect(circuitBreaker.getStats().successes).toBe(1);
    });

    it('should handle concurrent executions', async () => {
      const mockOperation = jest.fn().mockResolvedValue('success');

      const promises = Array.from({ length: 5 }, () => circuitBreaker.execute(mockOperation));
      const results = await Promise.all(promises);

      expect(results).toEqual(['success', 'success', 'success', 'success', 'success']);
      expect(circuitBreaker.getStats().successes).toBe(5);
    });
  });
});