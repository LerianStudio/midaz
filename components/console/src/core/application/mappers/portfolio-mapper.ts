import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { CreatePortfolioDto, PortfolioResponseDto } from '../dto/portfolio-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'
import { AccountMapper } from './account-mapper'
import { AccountEntity } from '@/core/domain/entities/account-entity'

export class PortfolioMapper {
  public static toDomain(dto: CreatePortfolioDto): PortfolioEntity {
    return {
      entityId: dto.entityId,
      name: dto.name,
      ledgerId: dto.ledgerId,
      organizationId: dto.organizationId,
      metadata: dto.metadata ?? {}
    }
  }

  public static toResponseDto(
    portfolio: PortfolioEntity
  ): PortfolioResponseDto {
    return {
      id: portfolio.id!,
      entityId: portfolio.entityId!,
      ledgerId: portfolio.ledgerId!,
      organizationId: portfolio.organizationId!,
      name: portfolio.name,
      metadata: portfolio.metadata ?? {},
      createdAt: portfolio.createdAt!,
      updatedAt: portfolio.updatedAt!,
      deletedAt: portfolio.deletedAt ?? null
    }
  }

  public static toPaginationResponseDto(
    result: PaginationEntity<PortfolioEntity>
  ): PaginationEntity<PortfolioResponseDto> {
    return PaginationMapper.toResponseDto(result, PortfolioMapper.toResponseDto)
  }

  public static toDtoWithAccounts(
    portfolio: PortfolioEntity,
    accounts: AccountEntity[]
  ): PortfolioResponseDto {
    return {
      ...PortfolioMapper.toResponseDto(portfolio),
      accounts: accounts.map(AccountMapper.toDto)
    }
  }
}
