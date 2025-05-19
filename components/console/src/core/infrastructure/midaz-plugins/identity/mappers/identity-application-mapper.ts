import { ApplicationEntity } from '@/core/domain/entities/application-entity'
import {
  IdentityApplicationDto,
  IdentityCreateApplicationDto
} from '../dto/identity-application-dto'
import { IdentityPaginationDto } from '../dto/identity-pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { IdentityPaginationMapper } from './identity-pagination-mapper'

export class IdentityApplicationMapper {
  public static toEntity(
    application: IdentityApplicationDto
  ): ApplicationEntity {
    return {
      id: application.id,
      name: application.name,
      description: application.description,
      clientId: application.clientId,
      clientSecret: application.clientSecret,
      createdAt: application.createdAt
    }
  }

  public static toCreateDto(
    application: ApplicationEntity
  ): IdentityCreateApplicationDto {
    return {
      name: application.name,
      description: application.description
    }
  }

  public static toPaginationEntity(
    result: IdentityPaginationDto<IdentityApplicationDto>
  ): PaginationEntity<ApplicationEntity> {
    return IdentityPaginationMapper.toResponseDto(
      result,
      IdentityApplicationMapper.toEntity
    )
  }
}
