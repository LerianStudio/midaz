export abstract class DeleteAccountsRepository {
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    accountId: string
  ) => Promise<void>
}
