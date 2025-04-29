import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import {
  MidazCreateSegmentDto,
  MidazSegmentDto,
  MidazUpdateSegmentDto
} from '../dto/midaz-segment-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'

export class MidazSegmentMapper {
  public static toCreateDto(asset: SegmentEntity): MidazCreateSegmentDto {
    return {
      name: asset.name,
      metadata: asset.metadata
    }
  }

  public static toUpdateDto(
    asset: Partial<SegmentEntity>
  ): MidazUpdateSegmentDto {
    return {
      name: asset.name,
      metadata: asset.metadata
    }
  }

  public static toEntity(asset: MidazSegmentDto): SegmentEntity {
    return {
      id: asset.id,
      organizationId: asset.organizationId,
      ledgerId: asset.ledgerId,
      name: asset.name,
      metadata: asset.metadata ?? {},
      createdAt: asset.createdAt,
      updatedAt: asset.updatedAt,
      deletedAt: asset.deletedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazSegmentDto>
  ): PaginationEntity<SegmentEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazSegmentMapper.toEntity
    )
  }
}
