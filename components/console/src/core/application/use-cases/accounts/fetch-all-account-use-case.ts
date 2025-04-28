import { PaginationDto } from '../../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { AccountMapper } from '../../mappers/account-mapper'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountResponseDto } from '../../dto/account-dto'
import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAllAccounts {
  execute: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationDto<AccountResponseDto>>
}

@injectable()
export class FetchAllAccountsUseCase implements FetchAllAccounts {
  constructor(
    @inject(AccountRepository)
    private readonly accountRepository: AccountRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<AccountResponseDto>> {
    const accountsResult: PaginationEntity<AccountEntity> =
      await this.accountRepository.fetchAll(
        organizationId,
        ledgerId,
        page,
        limit
      )

    return AccountMapper.toPaginationResponseDto(accountsResult)
  }
}
