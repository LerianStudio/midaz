import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import {
  MidazCreatePortfolioDto,
  MidazPortfolioDto,
  MidazUpdatePortfolioDto
} from '../dto/midaz-portfolio-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazPaginationMapper } from './midaz-pagination-mapper'

export class MidazPortfolioMapper {
  public static toCreateDto(
    portfolio: PortfolioEntity
  ): MidazCreatePortfolioDto {
    return {
      name: portfolio.name,
      entityId: portfolio.entityId,
      metadata: portfolio.metadata
    }
  }

  public static toUpdateDto(
    portfolio: Partial<PortfolioEntity>
  ): MidazUpdatePortfolioDto {
    return {
      name: portfolio.name,
      entityId: portfolio.entityId,
      metadata: portfolio.metadata
    }
  }

  public static toEntity(portfolio: MidazPortfolioDto): PortfolioEntity {
    return {
      id: portfolio.id,
      organizationId: portfolio.organizationId,
      ledgerId: portfolio.ledgerId,
      name: portfolio.name,
      entityId: portfolio.entityId!,
      metadata: portfolio.metadata ?? {},
      createdAt: portfolio.createdAt,
      updatedAt: portfolio.updatedAt,
      deletedAt: portfolio.deletedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazPortfolioDto>
  ): PaginationEntity<PortfolioEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazPortfolioMapper.toEntity
    )
  }
}
