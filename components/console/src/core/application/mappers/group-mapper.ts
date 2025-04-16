import { GroupsEntity } from '@/core/domain/entities/groups-entity'
import { GroupResponseDto } from '../dto/group-dto'

export class GroupMapper {
  static toResponseDto(group: GroupsEntity): GroupResponseDto {
    return {
      id: group.id!,
      name: group.name,
      createdAt: group.createdAt!
    }
  }
}
