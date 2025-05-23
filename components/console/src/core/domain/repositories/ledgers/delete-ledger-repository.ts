export abstract class DeleteLedgerRepository {
  abstract delete: (organizationId: string, ledgerId: string) => Promise<void>
}
