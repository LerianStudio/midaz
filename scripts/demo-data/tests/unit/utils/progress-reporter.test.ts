/**
 * Tests for progress reporter
 */

import { ProgressReporter } from '../../../src/utils/progress-reporter';
import { Logger } from '../../../src/services/logger';

// Mock the logger
jest.mock('../../../src/services/logger');

const createMockLogger = (): jest.Mocked<Logger> => {
  return {
    debug: jest.fn(),
    info: jest.fn(),
    warn: jest.fn(),
    error: jest.fn(),
    progress: jest.fn(),
  } as any;
};

describe('ProgressReporter', () => {
  let mockLogger: jest.Mocked<Logger>;
  let progressReporter: ProgressReporter;

  beforeEach(() => {
    mockLogger = createMockLogger();
    jest.clearAllMocks();
  });

  afterEach(() => {
    if (progressReporter) {
      progressReporter.stop();
    }
  });

  describe('initialization', () => {
    it('should initialize with correct values', () => {
      progressReporter = new ProgressReporter('TestEntity', 100, mockLogger);

      const metrics = progressReporter.getMetrics();
      expect(metrics.totalItems).toBe(100);
      expect(metrics.completedItems).toBe(0);
      expect(metrics.failedItems).toBe(0);
      expect(metrics.skippedItems).toBe(0);
      expect(metrics.throughputPerSecond).toBe(0);
    });

    it('should initialize with custom options', () => {
      progressReporter = new ProgressReporter('TestEntity', 50, mockLogger, {
        updateInterval: 5000,
        showETA: false,
        showThroughput: false,
        progressBarWidth: 50,
      });

      expect(progressReporter.getMetrics().totalItems).toBe(50);
    });
  });

  describe('progress tracking', () => {
    beforeEach(() => {
      progressReporter = new ProgressReporter('TestEntity', 10, mockLogger, {
        updateInterval: 0, // Disable automatic updates for testing
      });
    });

    it('should track completed items', () => {
      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted(100);

      const metrics = progressReporter.getMetrics();
      expect(metrics.completedItems).toBe(2);
      expect(metrics.failedItems).toBe(0);
      expect(metrics.skippedItems).toBe(0);
    });

    it('should track failed items', () => {
      progressReporter.reportItemFailed();
      progressReporter.reportItemFailed();

      const metrics = progressReporter.getMetrics();
      expect(metrics.completedItems).toBe(0);
      expect(metrics.failedItems).toBe(2);
      expect(metrics.skippedItems).toBe(0);
    });

    it('should track skipped items', () => {
      progressReporter.reportItemSkipped();
      progressReporter.reportItemSkipped();

      const metrics = progressReporter.getMetrics();
      expect(metrics.completedItems).toBe(0);
      expect(metrics.failedItems).toBe(0);
      expect(metrics.skippedItems).toBe(2);
    });

    it('should track batch completion', () => {
      progressReporter.reportBatchCompleted(5, 500);

      const metrics = progressReporter.getMetrics();
      expect(metrics.completedItems).toBe(5);
      expect(metrics.failedItems).toBe(0);
      expect(metrics.skippedItems).toBe(0);
    });

    it('should calculate throughput', async () => {
      progressReporter.start();
      
      // Simulate some progress
      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();
      
      // Wait a bit for time to pass
      await new Promise(resolve => setTimeout(resolve, 10));
      
      const metrics = progressReporter.getMetrics();
      expect(metrics.throughputPerSecond).toBeGreaterThan(0);
    });
  });

  describe('completion detection', () => {
    beforeEach(() => {
      progressReporter = new ProgressReporter('TestEntity', 5, mockLogger, {
        updateInterval: 0,
      });
    });

    it('should detect when generation is complete', () => {
      expect(progressReporter.isComplete()).toBe(false);

      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();

      expect(progressReporter.isComplete()).toBe(true);
    });

    it('should detect completion with mixed results', () => {
      expect(progressReporter.isComplete()).toBe(false);

      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();
      progressReporter.reportItemFailed();
      progressReporter.reportItemSkipped();
      progressReporter.reportItemCompleted();

      expect(progressReporter.isComplete()).toBe(true);
    });
  });

  describe('progress reporting with updates', () => {
    it('should start and log initial message', () => {
      progressReporter = new ProgressReporter('TestEntity', 10, mockLogger, {
        updateInterval: 0,
      });

      progressReporter.start();

      expect(mockLogger.info).toHaveBeenCalledWith(
        expect.stringContaining('🚀 Starting generation of 10 TestEntitys')
      );
    });

    it('should show final summary on stop', () => {
      progressReporter = new ProgressReporter('TestEntity', 5, mockLogger, {
        updateInterval: 0,
      });

      progressReporter.start();
      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();
      progressReporter.reportItemFailed();
      progressReporter.stop();

      // Check that final summary was logged
      expect(mockLogger.info).toHaveBeenCalledWith(
        expect.stringContaining('🎯 TestEntity Generation Complete!')
      );
      expect(mockLogger.info).toHaveBeenCalledWith(
        expect.stringContaining('✅ Successful: 2')
      );
      expect(mockLogger.warn).toHaveBeenCalledWith(
        expect.stringContaining('❌ Failed: 1')
      );
    });

    it('should show progress with progress bar when enabled', () => {
      progressReporter = new ProgressReporter('TestEntity', 10, mockLogger, {
        updateInterval: 0,
        showProgressBar: true,
        progressBarWidth: 10,
      });

      progressReporter.start();
      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();
      progressReporter.forceUpdate();

      // Should include progress bar in the output
      expect(mockLogger.info).toHaveBeenCalledWith(
        expect.stringMatching(/\[.*\]\s+\d+\.\d+%/)
      );
    });

    it('should show throughput when enabled', () => {
      progressReporter = new ProgressReporter('TestEntity', 10, mockLogger, {
        updateInterval: 0,
        showThroughput: true,
      });

      progressReporter.start();
      progressReporter.reportItemCompleted(100);
      progressReporter.reportItemCompleted(150);
      progressReporter.forceUpdate();

      // Should include throughput in the output
      expect(mockLogger.info).toHaveBeenCalledWith(
        expect.stringMatching(/🚀\s+\d+\.\d+\/s/)
      );
    });
  });

  describe('time calculations', () => {
    beforeEach(() => {
      progressReporter = new ProgressReporter('TestEntity', 10, mockLogger, {
        updateInterval: 0,
        showETA: true,
      });
    });

    it('should calculate average item time', () => {
      progressReporter.start();
      progressReporter.reportItemCompleted(100);
      progressReporter.reportItemCompleted(200);
      progressReporter.reportItemCompleted(300);

      const metrics = progressReporter.getMetrics();
      expect(metrics.averageItemTime).toBe(200); // (100 + 200 + 300) / 3
    });

    it('should calculate estimated completion time', async () => {
      progressReporter.start();
      
      // Add some completed items to establish a rate
      progressReporter.reportItemCompleted();
      progressReporter.reportItemCompleted();
      
      // Wait a bit for throughput calculation
      await new Promise(resolve => setTimeout(resolve, 10));
      
      const metrics = progressReporter.getMetrics();
      if (metrics.throughputPerSecond > 0) {
        expect(metrics.estimatedCompletion).toBeDefined();
        expect(metrics.estimatedCompletion).toBeInstanceOf(Date);
      }
    });
  });

  describe('batch processing', () => {
    beforeEach(() => {
      progressReporter = new ProgressReporter('TestEntity', 20, mockLogger, {
        updateInterval: 0,
      });
    });

    it('should handle batch completion correctly', () => {
      progressReporter.start();
      progressReporter.reportBatchCompleted(5, 1000);
      progressReporter.reportBatchCompleted(3, 600);

      const metrics = progressReporter.getMetrics();
      expect(metrics.completedItems).toBe(8);
      expect(metrics.averageItemTime).toBe(200); // ((1000/5) + (600/3)) / 2 = (200 + 200) / 2
    });

    it('should limit sample size for performance', () => {
      progressReporter.start();
      
      // Add more than max samples (100)
      for (let i = 0; i < 150; i++) {
        progressReporter.reportItemCompleted(100);
      }

      const metrics = progressReporter.getMetrics();
      expect(metrics.completedItems).toBe(150);
      expect(metrics.averageItemTime).toBe(100); // Should still work with limited samples
    });
  });

  describe('error handling', () => {
    it('should handle zero total items gracefully', () => {
      progressReporter = new ProgressReporter('TestEntity', 0, mockLogger);
      
      expect(progressReporter.isComplete()).toBe(true);
      expect(progressReporter.getMetrics().totalItems).toBe(0);
    });

    it('should handle negative update interval', () => {
      progressReporter = new ProgressReporter('TestEntity', 10, mockLogger, {
        updateInterval: -1000,
      });

      progressReporter.start();
      
      // Should not crash and should work without automatic updates
      expect(progressReporter.getMetrics().totalItems).toBe(10);
    });
  });
});