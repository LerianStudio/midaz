import { AccountEntity } from '../../entities/account-entity'

export abstract class UpdateAccountsRepository {
  abstract update: (
    organizationId: string,
    ledgerId: string,
    accountId: string,
    account: Partial<AccountEntity>
  ) => Promise<AccountEntity>
}
