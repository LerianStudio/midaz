import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetResponseDto } from '../dto/asset-response-dto'
import { CreateAssetDto } from '../dto/create-asset-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'

export class AssetMapper {
  public static toDomain(dto: CreateAssetDto): AssetEntity {
    return {
      name: dto.name!,
      type: dto.type!,
      code: dto.code!,
      status: dto.status!,
      metadata: dto.metadata!
    }
  }

  public static toResponseDto(entity: AssetEntity): AssetResponseDto {
    return {
      id: entity.id!,
      organizationId: entity.organizationId!,
      ledgerId: entity.ledgerId!,
      name: entity.name,
      type: entity.type,
      code: entity.code,
      status: {
        ...entity.status,
        description: entity.status.description ?? ''
      },
      metadata: entity.metadata,
      createdAt: entity.createdAt!,
      updatedAt: entity.updatedAt!,
      deletedAt: entity.deletedAt!
    }
  }

  public static toPaginationResponseDto(
    result: PaginationEntity<AssetEntity>
  ): PaginationEntity<AssetResponseDto> {
    return PaginationMapper.toResponseDto(result, AssetMapper.toResponseDto)
  }
}
