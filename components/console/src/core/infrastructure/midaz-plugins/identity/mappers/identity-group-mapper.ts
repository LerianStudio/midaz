import { GroupEntity } from '@/core/domain/entities/group-entity'
import { IdentityGroupDto } from '../dto/identity-group-dto'

export class IdentityGroupMapper {
  public static toEntity(group: IdentityGroupDto): GroupEntity {
    return {
      id: group.id,
      name: group.name,
      createdAt: group.createdAt
    }
  }
}
