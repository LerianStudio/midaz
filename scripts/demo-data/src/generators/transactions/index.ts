/**
 * Transaction generators exports
 */

// Legacy exports (to be removed later)
export { TransactionGenerator } from './transaction.generator';
export { DepositGenerator as LegacyDepositGenerator, type DepositResult, type DepositConfig } from './deposit.generator';
export { TransferGenerator as LegacyTransferGenerator, type TransferResult, type TransferConfig } from './transfer.generator';
export type { TransactionConfig } from './transaction.generator';

// New modular exports
export { TransactionOrchestrator } from './transaction-orchestrator';
export { DepositGenerator } from './deposit-generator';
export { TransferGenerator } from './transfer-generator';
export {
  TransactionStrategyFactory,
  AssetAwareDepositStrategy,
  AssetAwareTransferStrategy,
  type DepositStrategy,
  type TransferStrategy,
  type AccountWithAsset,
} from './strategies/transaction-strategies';
export {
  chunkArray,
  wait,
  groupAccountsByAsset,
  prepareAccountsForDeposits,
  calculateOptimalConcurrency,
  formatErrorMessage,
  extractUniqueErrorMessages,
} from './helpers/transaction-helpers';