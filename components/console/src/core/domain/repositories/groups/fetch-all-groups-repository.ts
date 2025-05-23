import { GroupsEntity } from '../../entities/groups-entity'

export abstract class FetchAllGroupsRepository {
  abstract fetchAllGroups(): Promise<GroupsEntity[]>
}
