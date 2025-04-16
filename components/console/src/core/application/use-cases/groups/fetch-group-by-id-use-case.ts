import { inject, injectable } from 'inversify'
import { GroupResponseDto } from '../../dto/group-dto'
import { FetchGroupByIdRepository } from '@/core/domain/repositories/groups/fetch-group-by-id-repository'
import { GroupMapper } from '../../mappers/group-mapper'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchGroupById {
  execute: (groupId: string) => Promise<GroupResponseDto>
}

@injectable()
export class FetchGroupByIdUseCase implements FetchGroupById {
  constructor(
    @inject(FetchGroupByIdRepository)
    private readonly fetchGroupByIdRepository: FetchGroupByIdRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(groupId: string): Promise<GroupResponseDto> {
    const groupEntity =
      await this.fetchGroupByIdRepository.fetchGroupById(groupId)

    const groupResponseDto: GroupResponseDto =
      GroupMapper.toResponseDto(groupEntity)

    return groupResponseDto
  }
}
