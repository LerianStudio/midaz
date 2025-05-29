import { DEFAULT_OPTIONS, VOLUME_METRICS } from '../../src/config';
import { VolumeSize } from '../../src/types';

describe('Config Module', () => {
  describe('DEFAULT_OPTIONS', () => {
    it('should have the expected default values', () => {
      expect(DEFAULT_OPTIONS).toHaveProperty('volume', VolumeSize.SMALL);
      expect(DEFAULT_OPTIONS).toHaveProperty('baseUrl');
      expect(DEFAULT_OPTIONS).toHaveProperty('onboardingPort');
      expect(DEFAULT_OPTIONS).toHaveProperty('transactionPort');
      expect(DEFAULT_OPTIONS).toHaveProperty('concurrency');
      expect(DEFAULT_OPTIONS).toHaveProperty('debug', false);
    });
  });

  describe('VOLUME_METRICS', () => {
    it('should define metrics for all volume sizes', () => {
      expect(VOLUME_METRICS).toHaveProperty(VolumeSize.SMALL);
      expect(VOLUME_METRICS).toHaveProperty(VolumeSize.MEDIUM);
      expect(VOLUME_METRICS).toHaveProperty(VolumeSize.LARGE);
    });

    it('should have higher values for larger volumes', () => {
      // Organizations
      expect(VOLUME_METRICS[VolumeSize.SMALL].organizations).toBeLessThan(
        VOLUME_METRICS[VolumeSize.MEDIUM].organizations
      );
      expect(VOLUME_METRICS[VolumeSize.MEDIUM].organizations).toBeLessThan(
        VOLUME_METRICS[VolumeSize.LARGE].organizations
      );

      // Ledgers
      expect(VOLUME_METRICS[VolumeSize.SMALL].ledgersPerOrg).toBeLessThanOrEqual(
        VOLUME_METRICS[VolumeSize.MEDIUM].ledgersPerOrg
      );
      expect(VOLUME_METRICS[VolumeSize.MEDIUM].ledgersPerOrg).toBeLessThanOrEqual(
        VOLUME_METRICS[VolumeSize.LARGE].ledgersPerOrg
      );
    });
  });
});
