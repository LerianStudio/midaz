import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazPaginationMapper } from './midaz-pagination-mapper'
import {
  MidazBalanceDto,
  MidazUpdateBalanceDto
} from '../dto/midaz-balance-dto'
import { BalanceEntity } from '@/core/domain/entities/balance-entity'

export class MidazBalanceMapper {
  public static toUpdateDto(
    balance: Partial<BalanceEntity>
  ): MidazUpdateBalanceDto {
    return {
      allowSending: balance.allowSending,
      allowReceiving: balance.allowReceiving
    }
  }

  public static toEntity(balance: MidazBalanceDto): BalanceEntity {
    return {
      id: balance.id,
      organizationId: balance.organizationId,
      ledgerId: balance.ledgerId,
      accountId: balance.accountId,
      alias: balance.alias,
      assetCode: balance.assetCode,
      available: balance.available,
      onHold: balance.onHold,
      allowSending: balance.allowSending,
      allowReceiving: balance.allowReceiving,
      createdAt: balance.createdAt,
      updatedAt: balance.updatedAt,
      deletedAt: balance.deletedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazBalanceDto>
  ): PaginationEntity<BalanceEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazBalanceMapper.toEntity
    )
  }
}
