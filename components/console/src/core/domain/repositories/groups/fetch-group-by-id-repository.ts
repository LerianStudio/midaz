import { GroupsEntity } from '../../entities/groups-entity'

export abstract class FetchGroupByIdRepository {
  abstract fetchGroupById(groupId: string): Promise<GroupsEntity>
}
