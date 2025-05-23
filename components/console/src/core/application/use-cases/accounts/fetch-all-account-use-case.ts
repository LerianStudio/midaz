import { PaginationDto } from '../../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { AccountMapper } from '../../mappers/account-mapper'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountResponseDto } from '../../dto/account-dto'
import { FetchAllAccountsRepository } from '@/core/domain/repositories/accounts/fetch-all-accounts-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(FetchAllAccountsRepository)
    private readonly fetchAllAccountsRepository: FetchAllAccountsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<AccountResponseDto>> {
    const accountsResult: PaginationEntity<AccountEntity> =
      await this.fetchAllAccountsRepository.fetchAll(
        organizationId,
        ledgerId,
        page,
        limit
      )

    return AccountMapper.toPaginationResponseDto(accountsResult)
  }
}
