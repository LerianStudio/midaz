/**
 * Tests for transaction generation strategies
 */

import {
  AssetAwareDepositStrategy,
  AssetAwareTransferStrategy,
  TransactionStrategyFactory,
  AccountWithAsset,
} from '../../../../src/generators/transactions/strategies/transaction-strategies';
import { GENERATOR_CONFIG } from '../../../../src/config/generator-config';

describe('Transaction Strategies', () => {
  describe('AssetAwareDepositStrategy', () => {
    let strategy: AssetAwareDepositStrategy;

    beforeEach(() => {
      strategy = new AssetAwareDepositStrategy();
    });

    describe('calculateAmount', () => {
      it('should use predefined deposit amount if available', () => {
        const account: AccountWithAsset = {
          accountId: 'acc1',
          accountAlias: 'alias1',
          assetCode: 'BRL',
          depositAmount: 500000,
        };

        expect(strategy.calculateAmount(account)).toBe(500000);
      });

      it('should return crypto amount for BTC', () => {
        const account: AccountWithAsset = {
          accountId: 'acc1',
          accountAlias: 'alias1',
          assetCode: 'BTC',
        };

        expect(strategy.calculateAmount(account)).toBe(GENERATOR_CONFIG.assets.deposits.CRYPTO);
      });

      it('should return crypto amount for ETH', () => {
        const account: AccountWithAsset = {
          accountId: 'acc1',
          accountAlias: 'alias1',
          assetCode: 'ETH',
        };

        expect(strategy.calculateAmount(account)).toBe(GENERATOR_CONFIG.assets.deposits.CRYPTO);
      });

      it('should return commodity amount for XAU', () => {
        const account: AccountWithAsset = {
          accountId: 'acc1',
          accountAlias: 'alias1',
          assetCode: 'XAU',
        };

        expect(strategy.calculateAmount(account)).toBe(GENERATOR_CONFIG.assets.deposits.COMMODITIES);
      });

      it('should return commodity amount for XAG', () => {
        const account: AccountWithAsset = {
          accountId: 'acc1',
          accountAlias: 'alias1',
          assetCode: 'XAG',
        };

        expect(strategy.calculateAmount(account)).toBe(GENERATOR_CONFIG.assets.deposits.COMMODITIES);
      });

      it('should return default amount for currencies', () => {
        const currencies = ['BRL', 'USD', 'EUR'];
        
        currencies.forEach(assetCode => {
          const account: AccountWithAsset = {
            accountId: 'acc1',
            accountAlias: 'alias1',
            assetCode,
          };

          expect(strategy.calculateAmount(account)).toBe(GENERATOR_CONFIG.assets.deposits.DEFAULT);
        });
      });
    });

    describe('getSourceAccount', () => {
      it('should format external account correctly', () => {
        expect(strategy.getSourceAccount('BRL')).toBe('@external/BRL');
        expect(strategy.getSourceAccount('USD')).toBe('@external/USD');
        expect(strategy.getSourceAccount('BTC')).toBe('@external/BTC');
      });
    });
  });

  describe('AssetAwareTransferStrategy', () => {
    let strategy: AssetAwareTransferStrategy;

    beforeEach(() => {
      strategy = new AssetAwareTransferStrategy();
    });

    describe('calculateAmount', () => {
      it('should return small crypto amounts when useSmallAmounts is true', () => {
        const range = strategy.calculateAmount('BTC', true);
        expect(range).toEqual(GENERATOR_CONFIG.assets.transfers.CRYPTO_SMALL);
      });

      it('should return regular crypto amounts when useSmallAmounts is false', () => {
        const range = strategy.calculateAmount('ETH', false);
        expect(range).toEqual(GENERATOR_CONFIG.assets.transfers.CRYPTO);
      });

      it('should return small commodity amounts when useSmallAmounts is true', () => {
        const range = strategy.calculateAmount('XAU', true);
        expect(range).toEqual(GENERATOR_CONFIG.assets.transfers.COMMODITIES_SMALL);
      });

      it('should return regular commodity amounts when useSmallAmounts is false', () => {
        const range = strategy.calculateAmount('XAG', false);
        expect(range).toEqual(GENERATOR_CONFIG.assets.transfers.COMMODITIES);
      });

      it('should return small currency amounts when useSmallAmounts is true', () => {
        const range = strategy.calculateAmount('BRL', true);
        expect(range).toEqual(GENERATOR_CONFIG.assets.transfers.CURRENCIES_SMALL);
      });

      it('should return regular currency amounts when useSmallAmounts is false', () => {
        const range = strategy.calculateAmount('USD', false);
        expect(range).toEqual(GENERATOR_CONFIG.assets.transfers.CURRENCIES);
      });
    });

    describe('selectTargetAccount', () => {
      const accounts: AccountWithAsset[] = [
        { accountId: 'acc1', accountAlias: 'alias1', assetCode: 'BRL' },
        { accountId: 'acc2', accountAlias: 'alias2', assetCode: 'BRL' },
        { accountId: 'acc3', accountAlias: 'alias3', assetCode: 'USD' },
        { accountId: 'acc4', accountAlias: 'alias4', assetCode: 'BRL' },
      ];

      it('should select account with same asset code', () => {
        const sourceAccount = accounts[0];
        const target = strategy.selectTargetAccount(sourceAccount, accounts);

        expect(target).toBeDefined();
        expect(target?.assetCode).toBe('BRL');
        expect(target?.accountId).not.toBe(sourceAccount.accountId);
      });

      it('should return null if no valid targets', () => {
        const sourceAccount = accounts[2]; // USD account
        const target = strategy.selectTargetAccount(sourceAccount, accounts);

        expect(target).toBeNull();
      });

      it('should not select the source account itself', () => {
        const sourceAccount = accounts[0];
        
        // Run multiple times to ensure randomness doesn't select source
        for (let i = 0; i < 10; i++) {
          const target = strategy.selectTargetAccount(sourceAccount, accounts);
          if (target) {
            expect(target.accountId).not.toBe(sourceAccount.accountId);
          }
        }
      });
    });
  });

  describe('TransactionStrategyFactory', () => {
    it('should create deposit strategy', () => {
      const strategy = TransactionStrategyFactory.createDepositStrategy();
      expect(strategy).toBeInstanceOf(AssetAwareDepositStrategy);
    });

    it('should create transfer strategy', () => {
      const strategy = TransactionStrategyFactory.createTransferStrategy();
      expect(strategy).toBeInstanceOf(AssetAwareTransferStrategy);
    });
  });
});