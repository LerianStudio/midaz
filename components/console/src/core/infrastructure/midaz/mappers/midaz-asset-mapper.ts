import { AssetEntity } from '@/core/domain/entities/asset-entity'
import {
  MidazAssetDto,
  MidazCreateAssetDto,
  MidazUpdateAssetDto
} from '../dto/midaz-asset-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazPaginationMapper } from './midaz-pagination-mapper'

export class MidazAssetMapper {
  public static toCreateDto(asset: AssetEntity): MidazCreateAssetDto {
    return {
      name: asset.name,
      type: asset.type,
      code: asset.code,
      metadata: asset.metadata
    }
  }

  public static toUpdateDto(asset: Partial<AssetEntity>): MidazUpdateAssetDto {
    return {
      name: asset.name,
      metadata: asset.metadata
    }
  }

  public static toEntity(asset: MidazAssetDto): AssetEntity {
    return {
      id: asset.id,
      organizationId: asset.organizationId,
      ledgerId: asset.ledgerId,
      name: asset.name,
      type: asset.type,
      code: asset.code,
      metadata: asset.metadata ?? {},
      createdAt: asset.createdAt,
      updatedAt: asset.updatedAt,
      deletedAt: asset.deletedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazAssetDto>
  ): PaginationEntity<AssetEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazAssetMapper.toEntity
    )
  }
}
