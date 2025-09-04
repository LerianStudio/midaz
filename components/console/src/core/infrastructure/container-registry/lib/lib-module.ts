import { IntlShape } from 'react-intl'
import { Container, ContainerModule } from '../../utils/di/container'
import { getIntl } from '@/lib/intl/get-intl'

export const TYPES = {
  Intl: Symbol.for('INTL')
}

export const LibModule = new ContainerModule((container: Container) => {
  container
    .bind<IntlShape>(TYPES.Intl)
    .toDynamicValue(async () => await getIntl())
})
