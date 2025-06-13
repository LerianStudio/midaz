import { BalanceEntity } from '@/core/domain/entities/balance-entity'
import { BalanceDto } from '../dto/balance-dto'
import { PaginationDto } from '../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'

export class BalanceMapper {
  public static toDomain(dto: any): BalanceEntity {
    return {
      allowSending: dto.allowSending!,
      allowReceiving: dto.allowReceiving!
    }
  }

  public static toResponseDto(entity: BalanceEntity): BalanceDto {
    return {
      id: entity.id!,
      organizationId: entity.organizationId!,
      ledgerId: entity.ledgerId!,
      accountId: entity.accountId!,
      alias: entity.alias!,
      assetCode: entity.assetCode!,
      available: entity.available!,
      onHold: entity.onHold!,
      allowSending: entity.allowSending!,
      allowReceiving: entity.allowReceiving!,
      createdAt: entity.createdAt!,
      updatedAt: entity.updatedAt!,
      deletedAt: entity.deletedAt
    }
  }

  public static toPaginationResponseDto(
    result: PaginationEntity<BalanceEntity>
  ): PaginationDto<BalanceDto> {
    return PaginationMapper.toResponseDto(result, BalanceMapper.toResponseDto)
  }
}
