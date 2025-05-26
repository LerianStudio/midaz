import { Container, ContainerModule } from '../../utils/di/container'

// Services
import { CrmHttpService } from '../../midaz-plugins/crm/services/crm-http-service'

// Repositories
import { HolderRepository } from '@/core/domain/repositories/crm/holder-repository'
import { CrmHolderRepository } from '../../midaz-plugins/crm/repositories/crm-holder-repository'
import { AliasRepository } from '@/core/domain/repositories/crm/alias-repository'
import { CrmAliasRepository } from '../../midaz-plugins/crm/repositories/crm-alias-repository'

// Create symbols for injection
export const CRM_SYMBOLS = {
  HolderRepository: Symbol.for('HolderRepository'),
  AliasRepository: Symbol.for('AliasRepository')
}

export const CrmPluginModule = new ContainerModule((container: Container) => {
  // Services
  container.bind<CrmHttpService>(CrmHttpService).toSelf().inSingletonScope()

  // Repositories
  container
    .bind<HolderRepository>(CRM_SYMBOLS.HolderRepository)
    .to(CrmHolderRepository)
  container
    .bind<AliasRepository>(CRM_SYMBOLS.AliasRepository)
    .to(CrmAliasRepository)
})
