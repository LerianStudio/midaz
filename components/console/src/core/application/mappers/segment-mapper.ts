import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import {
  CreateSegmentDto,
  SegmentResponseDto,
  UpdateSegmentDto
} from '../dto/segment-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'

export class SegmentMapper {
  static toDomain(dto: CreateSegmentDto | UpdateSegmentDto): SegmentEntity {
    return {
      name: dto.name!,
      status: dto.status!,
      metadata: dto.metadata!
    }
  }

  static toResponseDto(segment: SegmentEntity): SegmentResponseDto {
    return {
      id: segment.id!,
      organizationId: segment.organizationId!,
      ledgerId: segment.ledgerId!,
      name: segment.name,
      status: {
        code: segment.status.code,
        description: segment.status.description ?? ''
      },
      metadata: segment.metadata ?? {},
      createdAt: segment.createdAt!,
      updatedAt: segment.updatedAt!,
      deletedAt: segment.deletedAt!
    }
  }

  static toPaginationResponseDto(
    result: PaginationEntity<SegmentEntity>
  ): PaginationEntity<SegmentResponseDto> {
    return PaginationMapper.toResponseDto(result, SegmentMapper.toResponseDto)
  }
}
