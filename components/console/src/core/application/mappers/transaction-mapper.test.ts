import { TransactionMapper } from './transaction-mapper'
import { CreateTransactionDto } from '../dto/transaction-dto'
import {
  TransactionCreateEntity,
  TransactionEntity
} from '@/core/domain/entities/transaction-entity'

describe('TransactionMapper', () => {
  describe('toDomain', () => {
    it('should map CreateTransactionDto to TransactionEntity', () => {
      const dto: CreateTransactionDto = {
        asset: 'USD',
        value: 100,
        source: [
          {
            account: 'source-account-1',
            value: 50,
            asset: 'USD',
            metadata: { key: 'value' }
          }
        ],
        destination: [
          {
            account: 'destination-account-1',
            value: 50,
            asset: 'USD',
            metadata: { key: 'value' }
          }
        ],
        metadata: { key: 'value' }
      }

      const entity: TransactionCreateEntity = TransactionMapper.toDomain(dto)

      expect(entity).toEqual({
        send: {
          asset: 'USD',
          value: 100,
          scale: 0,
          source: {
            from: [
              {
                account: 'source-account-1',
                amount: {
                  asset: 'USD',
                  value: 50,
                  scale: 0
                },
                metadata: { key: 'value' }
              }
            ]
          },
          distribute: {
            to: [
              {
                account: 'destination-account-1',
                amount: {
                  asset: 'USD',
                  value: 50,
                  scale: 0
                },
                metadata: { key: 'value' }
              }
            ]
          }
        },
        metadata: { key: 'value' }
      })
    })
  })

  describe('toResponseDto', () => {
    it('should map TransactionEntity to TransactionResponseDto', () => {
      const entity: TransactionEntity = {
        id: 'transaction-id',
        description: 'description',
        template: 'template',
        status: {
          code: 'status-code',
          description: 'status-message'
        },
        amount: 100000,
        amountScale: 0,
        assetCode: 'USD',
        chartOfAccountsGroupName: 'chart-of-accounts',
        source: ['source-account-1'],
        destination: ['destination-account-1'],
        ledgerId: 'ledger-id',
        organizationId: 'organization-id',
        operations: [
          {
            id: 'operation-id',
            transactionId: 'transaction-id',
            description: 'description',
            type: 'type',
            assetCode: 'USD',
            chartOfAccounts: 'chart-of-accounts',
            amount: { amount: 100000, scale: 0 },
            balance: { available: 100000, onHold: 0, scale: 0 },
            balanceAfter: { available: 0, onHold: 0, scale: 0 },
            status: {
              code: 'status-code',
              description: 'status-message'
            },
            accountId: 'account-id',
            accountAlias: 'account-alias',
            organizationId: 'organization-id',
            ledgerId: 'ledger-id',
            metadata: { key: 'value' },
            createdAt: '2021-01-01T00:00:00Z',
            updatedAt: '2021-01-01T00:00:00Z',
            deletedAt: '2021-01-01T00:00:00Z'
          },
          {
            id: 'operation-id',
            transactionId: 'transaction-id',
            description: 'description',
            type: 'type',
            assetCode: 'USD',
            chartOfAccounts: 'chart-of-accounts',
            amount: { amount: 100000, scale: 0 },
            balance: { available: 100000, onHold: 0, scale: 0 },
            balanceAfter: { available: 0, onHold: 0, scale: 0 },
            status: {
              code: 'status-code',
              description: 'status-message'
            },
            accountId: 'account-id',
            accountAlias: 'account-alias',
            organizationId: 'organization-id',
            ledgerId: 'ledger-id',
            metadata: { key: 'value' },
            createdAt: '2021-01-01T00:00:00Z',
            updatedAt: '2021-01-01T00:00:00Z',
            deletedAt: '2021-01-01T00:00:00Z'
          }
        ],
        metadata: { key: 'value' },
        createdAt: '2021-01-01T00:00:00Z',
        updatedAt: '2021-01-01T00:00:00Z',
        deletedAt: '2021-01-01T00:00:00Z'
      }

      const responseDto = TransactionMapper.toResponseDto(entity)

      expect(responseDto).toEqual({
        ...entity
      })
    })
  })
})
