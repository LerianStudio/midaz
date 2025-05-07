import { AccountEntity } from '@/core/domain/entities/account-entity'
import {
  MidazAccountDto,
  MidazCreateAccountDto,
  MidazUpdateAccountDto
} from '../dto/midaz-account-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'

export class MidazAccountMapper {
  public static toCreateDto(account: AccountEntity): MidazCreateAccountDto {
    return {
      name: account.name,
      alias: account.alias,
      assetCode: account.assetCode,
      type: account.type,
      entityId: account.entityId,
      parentAccountId: account.parentAccountId,
      portfolioId: account.portfolioId,
      segmentId: account.segmentId,
      metadata: account.metadata
    }
  }

  public static toUpdateDto(
    account: Partial<AccountEntity>
  ): MidazUpdateAccountDto {
    return {
      name: account.name,
      alias: account.alias,
      assetCode: account.assetCode,
      type: account.type,
      entityId: account.entityId,
      parentAccountId: account.parentAccountId,
      portfolioId: account.portfolioId,
      segmentId: account.segmentId,
      metadata: account.metadata
    }
  }

  public static toEntity(asset: MidazAccountDto): AccountEntity {
    return {
      id: asset.id,
      organizationId: asset.organizationId,
      ledgerId: asset.ledgerId,
      name: asset.name,
      type: asset.type,
      alias: asset.alias,
      assetCode: asset.assetCode,
      metadata: asset.metadata ?? {},
      createdAt: asset.createdAt,
      updatedAt: asset.updatedAt,
      deletedAt: asset.deletedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazAccountDto>
  ): PaginationEntity<AccountEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazAccountMapper.toEntity
    )
  }
}
