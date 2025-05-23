export abstract class DeleteOrganizationRepository {
  abstract deleteOrganization: (organizationId: string) => Promise<void>
}
