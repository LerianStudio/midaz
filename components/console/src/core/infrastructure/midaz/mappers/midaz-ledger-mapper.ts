import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import {
  MidazCreateLedgerDto,
  MidazLedgerDto,
  MidazUpdateLedgerDto
} from '../dto/midaz-ledger-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazPaginationMapper } from './midaz-pagination-mapper'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'

export class MidazLedgerMapper {
  public static toCreateDto(ledger: LedgerEntity): MidazCreateLedgerDto {
    return {
      name: ledger.name,
      metadata: ledger.metadata
    }
  }

  public static toUpdateDto(
    ledger: Partial<LedgerEntity>
  ): MidazUpdateLedgerDto {
    return {
      name: ledger.name,
      metadata: ledger.metadata
    }
  }

  public static toEntity(ledger: MidazLedgerDto): LedgerEntity {
    return {
      id: ledger.id,
      name: ledger.name,
      metadata: ledger.metadata ?? {},
      organizationId: ledger.organizationId,
      createdAt: ledger.createdAt,
      updatedAt: ledger.updatedAt,
      deletedAt: ledger.deletedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazLedgerDto>
  ): PaginationEntity<LedgerEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazLedgerMapper.toEntity
    )
  }
}
