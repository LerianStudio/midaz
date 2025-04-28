import { AccountEntity } from '../../entities/account-entity'

export abstract class FetchAccountByIdRepository {
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    accountId: string
  ) => Promise<AccountEntity>
}
