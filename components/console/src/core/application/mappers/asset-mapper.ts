import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetDto } from '../dto/asset-dto'
import { CreateAssetDto } from '../dto/asset-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'

export class AssetMapper {
  public static toDomain(dto: CreateAssetDto): AssetEntity {
    return {
      name: dto.name!,
      type: dto.type!,
      code: dto.code!,
      metadata: dto.metadata!
    }
  }

  public static toResponseDto(entity: AssetEntity): AssetDto {
    return {
      id: entity.id!,
      organizationId: entity.organizationId!,
      ledgerId: entity.ledgerId!,
      name: entity.name,
      type: entity.type,
      code: entity.code,
      metadata: entity.metadata,
      createdAt: entity.createdAt!,
      updatedAt: entity.updatedAt!,
      deletedAt: entity.deletedAt!
    }
  }

  public static toPaginationResponseDto(
    result: PaginationEntity<AssetEntity>
  ): PaginationEntity<AssetDto> {
    return PaginationMapper.toResponseDto(result, AssetMapper.toResponseDto)
  }
}
