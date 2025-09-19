import { MidazMenuGroupDto } from '@/core/application/dto/midaz-menu-dto'
import { MenuConfigService } from '@/core/infrastructure/config/services/menu-config-service'
import { LogOperation } from '@/core/infrastructure/logger/decorators'
import { inject, injectable } from 'inversify'

export abstract class GetMidazMenus {
  abstract execute(): Promise<MidazMenuGroupDto[]>
}

@injectable()
export class GetMidazMenusUseCase implements GetMidazMenus {
  constructor(
    @inject(MenuConfigService)
    private readonly menuConfigService: MenuConfigService
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<MidazMenuGroupDto[]> {
    return this.menuConfigService.getMenuConfig()
  }
}
