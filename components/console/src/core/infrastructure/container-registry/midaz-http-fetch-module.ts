import { Container, ContainerModule } from '../utils/di/container'
import { HttpFetchUtils } from '../utils/http-fetch-utils'
export const ContainerTypeMidazHttpFetch = {
  HttpFetchUtils: 'HttpFetchUtils'
}
export const MidazHttpFetchModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<HttpFetchUtils>(ContainerTypeMidazHttpFetch.HttpFetchUtils)
      .to(HttpFetchUtils)
  }
)
