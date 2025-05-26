import { inject, injectable } from 'inversify'
import { GroupResponseDto } from '../../dto/group-dto'
import { GroupRepository } from '@/core/domain/repositories/group-repository'
import { GroupMapper } from '../../mappers/group-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAllGroups {
  execute: () => Promise<GroupResponseDto[]>
}

@injectable()
export class FetchAllGroupsUseCase implements FetchAllGroups {
  constructor(
    @inject(GroupRepository)
    private readonly groupRepository: GroupRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<GroupResponseDto[]> {
    const groupsEntity = await this.groupRepository.fetchAll()

    const groupsResponseDto: GroupResponseDto[] = groupsEntity.map(
      GroupMapper.toResponseDto
    )

    return groupsResponseDto
  }
}
