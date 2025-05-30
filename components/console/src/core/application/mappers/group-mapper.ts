import { GroupEntity } from '@/core/domain/entities/group-entity'
import { GroupResponseDto } from '../dto/group-dto'

export class GroupMapper {
  static toResponseDto(group: GroupEntity): GroupResponseDto {
    return {
      id: group.id!,
      name: group.name,
      createdAt: group.createdAt!
    }
  }
}
