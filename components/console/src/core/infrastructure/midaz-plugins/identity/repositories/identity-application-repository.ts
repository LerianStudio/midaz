import { ApplicationEntity } from '@/core/domain/entities/application-entity'
import { ApplicationRepository } from '@/core/domain/repositories/identity/application-repository'
import { inject, injectable } from 'inversify'
import { IdentityApplicationDto } from '../dto/identity-application-dto'
import { IdentityApplicationMapper } from '../mappers/identity-application-mapper'
import { IdentityHttpService } from '../services/identity-http-service'

@injectable()
export class IdentityApplicationRepository implements ApplicationRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(IdentityHttpService)
    private readonly httpService: IdentityHttpService
  ) {}

  async fetchAll(): Promise<ApplicationEntity[]> {
    const response = await this.httpService.get<IdentityApplicationDto[]>(
      `${this.baseUrl}/applications`
    )

    return response.map((application) =>
      IdentityApplicationMapper.toEntity(application)
    )
  }

  async fetchById(applicationId: string): Promise<ApplicationEntity> {
    const response = await this.httpService.get<IdentityApplicationDto>(
      `${this.baseUrl}/applications/${applicationId}`
    )
    return IdentityApplicationMapper.toEntity(response)
  }

  async create(application: ApplicationEntity): Promise<ApplicationEntity> {
    const dto = IdentityApplicationMapper.toCreateDto(application)
    const response = await this.httpService.post<IdentityApplicationDto>(
      `${this.baseUrl}/applications`,
      {
        body: JSON.stringify(dto)
      }
    )
    return IdentityApplicationMapper.toEntity(response)
  }

  async delete(applicationId: string): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/applications/${applicationId}`
    )
  }
}
