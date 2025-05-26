import { ApplicationEntity } from '@/core/domain/entities/application-entity'
import {
  ApplicationResponseDto,
  CreateApplicationDto
} from '../dto/application-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'

export class ApplicationMapper {
  static toDomain(application: CreateApplicationDto): ApplicationEntity {
    return {
      name: application.name,
      description: application.description
    }
  }

  static toResponseDto(application: ApplicationEntity): ApplicationResponseDto {
    return {
      id: application.id!,
      name: application.name,
      description: application.description,
      clientId: application.clientId!,
      clientSecret: application.clientSecret!,
      createdAt: application.createdAt!
    }
  }

  static toPaginationResponseDto(
    result: PaginationEntity<ApplicationEntity>
  ): PaginationEntity<ApplicationResponseDto> {
    return PaginationMapper.toResponseDto(
      result,
      ApplicationMapper.toResponseDto
    )
  }
}
