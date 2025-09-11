import { CursorPaginationDto } from '../../dto/pagination-dto'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import { AccountTypesMapper } from '../../mappers/account-types-mapper'
import { AccountTypesEntity } from '@/core/domain/entities/account-types-entity'
import {
  AccountTypesDto,
  type AccountTypesSearchParamDto
} from '../../dto/account-types-dto'
import { AccountTypesRepository } from '@/core/domain/repositories/account-types-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface FetchAllAccountTypes {
  execute: (
    organizationId: string,
    ledgerId: string,
    query?: AccountTypesSearchParamDto
  ) => Promise<CursorPaginationDto<AccountTypesDto>>
}

@injectable()
export class FetchAllAccountTypesUseCase implements FetchAllAccountTypes {
  constructor(
    @inject(AccountTypesRepository)
    private readonly accountTypesRepository: AccountTypesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    query?: AccountTypesSearchParamDto
  ): Promise<CursorPaginationDto<AccountTypesDto>> {
    const searchEntity = AccountTypesMapper.toSearchDomain(query || {})

    const accountTypesResult: CursorPaginationEntity<AccountTypesEntity> =
      await this.accountTypesRepository.fetchAll(
        organizationId,
        ledgerId,
        searchEntity
      )

    return AccountTypesMapper.toCursorPaginationResponseDto(accountTypesResult)
  }
}
