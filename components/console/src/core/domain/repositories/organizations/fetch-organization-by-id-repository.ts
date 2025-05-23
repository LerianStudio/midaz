import { OrganizationEntity } from '../../entities/organization-entity'

export abstract class FetchOrganizationByIdRepository {
  abstract fetchById: (id: string) => Promise<OrganizationEntity>
}
