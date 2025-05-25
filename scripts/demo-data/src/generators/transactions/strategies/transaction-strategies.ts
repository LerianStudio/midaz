/**
 * Transaction generation strategies
 */

import { GENERATOR_CONFIG } from '../../../config/generator-config';

/**
 * Account interface for transaction processing
 */
export interface AccountWithAsset {
  accountId: string;
  accountAlias: string;
  assetCode: string;
  depositAmount?: number;
}

/**
 * Strategy for calculating deposit amounts
 */
export interface DepositStrategy {
  calculateAmount(account: AccountWithAsset): number;
  getSourceAccount(assetCode: string): string;
}

/**
 * Strategy for calculating transfer amounts
 */
export interface TransferStrategy {
  calculateAmount(assetCode: string, useSmallAmounts?: boolean): { min: number; max: number };
  selectTargetAccount(
    sourceAccount: AccountWithAsset,
    availableAccounts: AccountWithAsset[]
  ): AccountWithAsset | null;
}

/**
 * Asset-aware deposit strategy implementation
 */
export class AssetAwareDepositStrategy implements DepositStrategy {
  calculateAmount(account: AccountWithAsset): number {
    const assetCode = account.assetCode;

    // Use predefined deposit amount if available
    if (account.depositAmount !== undefined) {
      return account.depositAmount;
    }

    // Calculate based on asset type
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      return GENERATOR_CONFIG.assets.deposits.CRYPTO;
    } else if (assetCode === 'XAU' || assetCode === 'XAG') {
      return GENERATOR_CONFIG.assets.deposits.COMMODITIES;
    } else {
      return GENERATOR_CONFIG.assets.deposits.DEFAULT;
    }
  }

  getSourceAccount(assetCode: string): string {
    return GENERATOR_CONFIG.accounts.externalAccountFormat.replace('{assetCode}', assetCode);
  }
}

/**
 * Asset-aware transfer strategy implementation
 */
export class AssetAwareTransferStrategy implements TransferStrategy {
  calculateAmount(assetCode: string, useSmallAmounts: boolean = true): { min: number; max: number } {
    const transferConfig = GENERATOR_CONFIG.assets.transfers;

    if (assetCode === 'BTC' || assetCode === 'ETH') {
      return useSmallAmounts ? transferConfig.CRYPTO_SMALL : transferConfig.CRYPTO;
    } else if (assetCode === 'XAU' || assetCode === 'XAG') {
      return useSmallAmounts ? transferConfig.COMMODITIES_SMALL : transferConfig.COMMODITIES;
    } else {
      return useSmallAmounts ? transferConfig.CURRENCIES_SMALL : transferConfig.CURRENCIES;
    }
  }

  selectTargetAccount(
    sourceAccount: AccountWithAsset,
    availableAccounts: AccountWithAsset[]
  ): AccountWithAsset | null {
    // Filter accounts with same asset code, excluding source
    const validTargets = availableAccounts.filter(
      (acc) => acc.assetCode === sourceAccount.assetCode && acc.accountId !== sourceAccount.accountId
    );

    if (validTargets.length === 0) {
      return null;
    }

    // Select random target
    const targetIndex = Math.floor(Math.random() * validTargets.length);
    return validTargets[targetIndex];
  }
}

/**
 * Factory for creating strategies
 */
export class TransactionStrategyFactory {
  static createDepositStrategy(): DepositStrategy {
    return new AssetAwareDepositStrategy();
  }

  static createTransferStrategy(): TransferStrategy {
    return new AssetAwareTransferStrategy();
  }
}