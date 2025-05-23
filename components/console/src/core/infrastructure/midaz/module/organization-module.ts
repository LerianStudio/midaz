import { Container, ContainerModule } from '../../utils/di/container'

import { CreateOrganizationRepository } from '@/core/domain/repositories/organizations/create-organization-repository'
import { DeleteOrganizationRepository } from '@/core/domain/repositories/organizations/delete-organization-repository'
import { FetchAllOrganizationsRepository } from '@/core/domain/repositories/organizations/fetch-all-organizations-repository'
import { FetchOrganizationByIdRepository } from '@/core/domain/repositories/organizations/fetch-organization-by-id-repository'
import { UpdateOrganizationRepository } from '@/core/domain/repositories/organizations/update-organization-repository'

import { MidazCreateOrganizationRepository } from '../organizations/midaz-create-organization-repository'
import { MidazDeleteOrganizationRepository } from '../organizations/midaz-delete-organization-repository'
import { MidazFetchAllOrganizationsRepository } from '../organizations/midaz-fetch-all-organizations-repository'
import { MidazFetchOrganizationByIdRepository } from '../organizations/midaz-fetch-organization-by-id-repository'
import { MidazUpdateOrganizationRepository } from '../organizations/midaz-update-organization-repository'

export const MidazOrganizationModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<CreateOrganizationRepository>(CreateOrganizationRepository)
      .to(MidazCreateOrganizationRepository)

    container
      .bind<FetchAllOrganizationsRepository>(FetchAllOrganizationsRepository)
      .to(MidazFetchAllOrganizationsRepository)

    container
      .bind<FetchOrganizationByIdRepository>(FetchOrganizationByIdRepository)
      .to(MidazFetchOrganizationByIdRepository)

    container
      .bind<DeleteOrganizationRepository>(DeleteOrganizationRepository)
      .to(MidazDeleteOrganizationRepository)

    container
      .bind<UpdateOrganizationRepository>(UpdateOrganizationRepository)
      .to(MidazUpdateOrganizationRepository)
  }
)
