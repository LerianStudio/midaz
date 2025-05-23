import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { CreateLedgerDto } from '../dto/ledger-dto'
import { LedgerResponseDto } from '../dto/ledger-dto'
import { UpdateSegmentDto } from '../dto/segment-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'

export class LedgerMapper {
  public static toDomain(
    dto: CreateLedgerDto | UpdateSegmentDto
  ): LedgerEntity {
    return {
      name: dto.name!,
      metadata: dto.metadata!
    }
  }

  public static toResponseDto(ledger: LedgerEntity): LedgerResponseDto {
    return {
      id: ledger.id!,
      organizationId: ledger.organizationId!,
      name: ledger.name,
      metadata: ledger.metadata ?? {},
      createdAt: ledger.createdAt!,
      updatedAt: ledger.updatedAt!,
      deletedAt: ledger.deletedAt!
    }
  }

  public static toPaginationResponseDto(
    result: PaginationEntity<LedgerEntity>
  ): PaginationEntity<LedgerResponseDto> {
    return PaginationMapper.toResponseDto(result, LedgerMapper.toResponseDto)
  }
}
