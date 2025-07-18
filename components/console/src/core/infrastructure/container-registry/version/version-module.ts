import { Container, ContainerModule } from '../../utils/di/container'
import { VersionRepository } from '@/core/domain/repositories/version-repository'
import { DockerVersionRepository } from '../../version/repositories/docker-version-repository'

export const VersionModule = new ContainerModule((container: Container) => {
  container
    .bind<VersionRepository>(VersionRepository)
    .to(DockerVersionRepository)
})
