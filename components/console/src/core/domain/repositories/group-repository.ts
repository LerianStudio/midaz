import { GroupsEntity } from '../entities/groups-entity'

export abstract class GroupRepository {
  abstract fetchAll(): Promise<GroupsEntity[]>
  abstract fetchById(groupId: string): Promise<GroupsEntity>
}
