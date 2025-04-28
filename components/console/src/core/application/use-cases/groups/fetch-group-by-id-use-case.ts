import { inject, injectable } from 'inversify'
import { GroupResponseDto } from '../../dto/group-dto'
import { GroupRepository } from '@/core/domain/repositories/group-repository'
import { GroupMapper } from '../../mappers/group-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchGroupById {
  execute: (groupId: string) => Promise<GroupResponseDto>
}

@injectable()
export class FetchGroupByIdUseCase implements FetchGroupById {
  constructor(
    @inject(GroupRepository)
    private readonly groupRepository: GroupRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(groupId: string): Promise<GroupResponseDto> {
    const groupEntity = await this.groupRepository.fetchById(groupId)

    const groupResponseDto: GroupResponseDto =
      GroupMapper.toResponseDto(groupEntity)

    return groupResponseDto
  }
}
