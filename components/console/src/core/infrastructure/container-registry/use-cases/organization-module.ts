import {
  CreateOrganization,
  CreateOrganizationUseCase
} from '@/core/application/use-cases/organizations/create-organization-use-case'
import {
  DeleteOrganization,
  DeleteOrganizationUseCase
} from '@/core/application/use-cases/organizations/delete-organization-use-case'
import {
  FetchAllOrganizations,
  FetchAllOrganizationsUseCase
} from '@/core/application/use-cases/organizations/fetch-all-organizations-use-case'
import {
  FetchOrganizationById,
  FetchOrganizationByIdUseCase
} from '@/core/application/use-cases/organizations/fetch-organization-by-id-use-case'
import {
  FetchParentOrganizations,
  FetchParentOrganizationsUseCase
} from '@/core/application/use-cases/organizations/fetch-parent-organizations-use-case'
import {
  UpdateOrganization,
  UpdateOrganizationUseCase
} from '@/core/application/use-cases/organizations/update-organization-use-case'
import { Container, ContainerModule } from '../../utils/di/container'

export const OrganizationUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<CreateOrganization>(CreateOrganizationUseCase).toSelf()
    container.bind<FetchAllOrganizations>(FetchAllOrganizationsUseCase).toSelf()
    container.bind<FetchOrganizationById>(FetchOrganizationByIdUseCase).toSelf()
    container.bind<UpdateOrganization>(UpdateOrganizationUseCase).toSelf()
    container.bind<DeleteOrganization>(DeleteOrganizationUseCase).toSelf()
    container
      .bind<FetchParentOrganizations>(FetchParentOrganizationsUseCase)
      .toSelf()
  }
)
