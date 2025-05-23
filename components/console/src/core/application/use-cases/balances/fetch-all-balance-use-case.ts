import { inject, injectable } from 'inversify'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { BalanceDto } from '@/core/application/dto/balance-dto'
import { BalanceMapper } from '@/core/application/mappers/balance-mapper'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface FetchBalanceByAccountId {
  execute: (
    organizationId: string,
    ledgerId: string,
    accountId: string
  ) => Promise<PaginationDto<BalanceDto>>
}

@injectable()
export class FetchBalanceByAccountIdUseCase implements FetchBalanceByAccountId {
  constructor(
    @inject(BalanceRepository)
    private readonly balanceRepository: BalanceRepository
  ) {}

  @LogOperation({
    layer: 'application'
  })
  async execute(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<PaginationDto<BalanceDto>> {
    const response = await this.balanceRepository.fetchAll(
      organizationId,
      ledgerId,
      accountId
    )

    return BalanceMapper.toPaginationResponseDto(response)
  }
}
