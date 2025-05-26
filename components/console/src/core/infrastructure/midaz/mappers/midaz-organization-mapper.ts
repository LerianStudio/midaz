import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import {
  MidazCreateOrganizationDto,
  MidazOrganizationDto,
  MidazUpdateOrganizationDto
} from '../dto/midaz-organization-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { isNil, omitBy } from 'lodash'

export class MidazOrganizationMapper {
  public static toCreateDto(
    organization: OrganizationEntity
  ): MidazCreateOrganizationDto {
    return {
      legalName: organization.legalName,
      parentOrganizationId: organization.parentOrganizationId,
      doingBusinessAs: organization.doingBusinessAs,
      legalDocument: organization.legalDocument,
      address: {
        line1: organization.address.line1,
        line2: organization.address.line2,
        neighborhood: organization.address.neighborhood,
        city: organization.address.city,
        state: organization.address.state,
        country: organization.address.country,
        zipCode: organization.address.zipCode
      },
      metadata: organization.metadata
    }
  }

  public static toUpdateDto(
    organization: Partial<OrganizationEntity>
  ): MidazUpdateOrganizationDto {
    return omitBy(
      {
        legalName: organization.legalName,
        parentOrganizationId: organization.parentOrganizationId,
        legalDocument: organization.legalDocument,
        address: {
          line1: organization.address?.line1,
          line2: organization.address?.line2,
          neighborhood: organization.address?.neighborhood,
          city: organization.address?.city,
          state: organization.address?.state,
          country: organization.address?.country,
          zipCode: organization.address?.zipCode
        },
        metadata: organization.metadata
      },
      isNil
    )
  }

  public static toEntity(
    organization: MidazOrganizationDto
  ): OrganizationEntity {
    return {
      id: organization.id,
      legalName: organization.legalName,
      parentOrganizationId: organization.parentOrganizationId,
      doingBusinessAs: organization.doingBusinessAs,
      legalDocument: organization.legalDocument,
      address: {
        line1: organization.address.line1,
        line2: organization.address.line2,
        neighborhood: organization.address.neighborhood,
        city: organization.address.city,
        state: organization.address.state,
        country: organization.address.country,
        zipCode: organization.address.zipCode
      },
      metadata: organization.metadata ?? {},
      createdAt: organization.createdAt,
      updatedAt: organization.updatedAt,
      deletedAt: organization.deletedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazOrganizationDto>
  ): PaginationEntity<OrganizationEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazOrganizationMapper.toEntity
    )
  }
}
