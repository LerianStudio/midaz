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
      name: asset.name,
      parentAccountId: asset.parentAccountId,
      entityId: asset.entityId,
      assetCode: asset.assetCode,
      organizationId: asset.organizationId,
      ledgerId: asset.ledgerId,
      portfolioId: asset.portfolioId,
      segmentId: asset.segmentId,
      alias: asset.alias,
      type: asset.type,
      createdAt: asset.createdAt,
      updatedAt: asset.updatedAt,
      deletedAt: asset.deletedAt,
      metadata: asset.metadata ?? {}
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
