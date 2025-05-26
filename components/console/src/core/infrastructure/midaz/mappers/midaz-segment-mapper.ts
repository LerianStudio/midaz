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
  public static toCreateDto(segment: SegmentEntity): MidazCreateSegmentDto {
    return {
      name: segment.name,
      metadata: segment.metadata
    }
  }

  public static toUpdateDto(
    segment: Partial<SegmentEntity>
  ): MidazUpdateSegmentDto {
    return {
      name: segment.name,
      metadata: segment.metadata
    }
  }

  public static toEntity(segment: MidazSegmentDto): SegmentEntity {
    return {
      id: segment.id,
      organizationId: segment.organizationId,
      ledgerId: segment.ledgerId,
      name: segment.name,
      metadata: segment.metadata ?? {},
      createdAt: segment.createdAt,
      updatedAt: segment.updatedAt,
      deletedAt: segment.deletedAt
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
