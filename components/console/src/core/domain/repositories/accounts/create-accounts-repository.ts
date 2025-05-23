import { AccountEntity } from '../../entities/account-entity'

export abstract class CreateAccountsRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    account: AccountEntity
  ) => Promise<AccountEntity>
}
