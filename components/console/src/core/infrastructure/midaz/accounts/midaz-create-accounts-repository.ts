import { AccountEntity } from '@/core/domain/entities/account-entity'
import { CreateAccountsRepository } from '@/core/domain/repositories/accounts/create-accounts-repository'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils, HTTP_METHODS } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazCreateAccountRepository implements CreateAccountsRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    account: AccountEntity
  ): Promise<AccountEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<AccountEntity>({
        url,
        method: HTTP_METHODS.POST,
        body: JSON.stringify(account)
      })

    return response
  }
}
