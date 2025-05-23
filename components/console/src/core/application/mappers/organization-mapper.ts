import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { CreateOrganizationDto } from '../dto/organization-dto'
import { OrganizationResponseDto } from '../dto/organization-dto'
import { PaginationMapper } from './pagination-mapper'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { OrganizationAvatarEntity } from '@/core/domain/entities/organization-avatar-entity'

export class OrganizationMapper {
  public static toDomain(dto: CreateOrganizationDto): OrganizationEntity {
    return {
      legalName: dto.legalName!,
      parentOrganizationId: dto.parentOrganizationId,
      doingBusinessAs: dto.doingBusinessAs!,
      legalDocument: dto.legalDocument!,
      address: dto.address!,
      metadata: dto.metadata!
    }
  }

  public static toResponseDto(
    entity: OrganizationEntity,
    avatar?: string
  ): OrganizationResponseDto {
    return {
      id: entity.id!,
      legalName: entity.legalName,
      parentOrganizationId: entity.parentOrganizationId!,
      doingBusinessAs: entity.doingBusinessAs,
      legalDocument: entity.legalDocument,
      address: entity.address,
      avatar,
      metadata: entity.metadata ?? {},
      createdAt: entity.createdAt!,
      updatedAt: entity.updatedAt!,
      deletedAt: entity.deletedAt
    }
  }

  public static toPaginationResponseDto(
    result: PaginationEntity<OrganizationEntity>,
    organizationAvatar?: OrganizationAvatarEntity[]
  ): PaginationEntity<OrganizationResponseDto> {
    const items = result.items.map((item) => {
      if (!organizationAvatar) {
        return OrganizationMapper.toResponseDto(item)
      }

      const avatar = organizationAvatar.find(
        (avatar) => avatar.organizationId === item.id
      )

      return OrganizationMapper.toResponseDto(item, avatar?.avatar)
    })

    return {
      items,
      limit: result.limit,
      page: result.page
    }
  }
}
