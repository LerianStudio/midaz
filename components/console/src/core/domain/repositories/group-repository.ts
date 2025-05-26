import { GroupEntity } from '../entities/group-entity'

export abstract class GroupRepository {
  abstract fetchAll(): Promise<GroupEntity[]>
  abstract fetchById(groupId: string): Promise<GroupEntity>
}
