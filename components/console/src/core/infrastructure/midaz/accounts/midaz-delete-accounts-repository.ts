import { injectable, inject } from 'inversify'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'
import { DeleteAccountsRepository } from '@/core/domain/repositories/accounts/delete-accounts-repository'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazDeleteAccountsRepository implements DeleteAccountsRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async delete(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`
    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HTTP_METHODS.DELETE
    })

    return
  }
}
