import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { FetchAccountsWithPortfoliosUseCase } from '../use-cases/accounts-with-portfolios/fetch-accounts-with-portfolios-use-case'
import { NextResponse } from 'next/server'
import { FetchAccountByIdUseCase } from '../use-cases/accounts/fetch-account-by-id-use-case'
import { FetchAllAccountsUseCase } from '../use-cases/accounts/fetch-all-account-use-case'
import { CreateAccountUseCase } from '../use-cases/accounts/create-account-use-case'
import { DeleteAccountUseCase } from '../use-cases/accounts/delete-account-use-case'
import { UpdateAccountUseCase } from '../use-cases/accounts/update-account-use-case'
import { BaseController } from '@/lib/http/server/base-controller'

type AccountParams = {
  id: string
  ledgerId: string
  accountId?: string
}

@LoggerInterceptor()
@Controller()
export class AccountController extends BaseController {
  constructor(
    @inject(FetchAllAccountsUseCase)
    private readonly fetchAllAccountsUseCase: FetchAllAccountsUseCase,
    @inject(FetchAccountByIdUseCase)
    private readonly fetchAccountByIdUseCase: FetchAccountByIdUseCase,
    @inject(CreateAccountUseCase)
    private readonly createAccountUseCase: CreateAccountUseCase,
    @inject(UpdateAccountUseCase)
    private readonly updateAccountUseCase: UpdateAccountUseCase,
    @inject(DeleteAccountUseCase)
    private readonly deleteAccountUseCase: DeleteAccountUseCase,
    @inject(FetchAccountsWithPortfoliosUseCase)
    private readonly fetchAccountsWithPortfoliosUseCase: FetchAccountsWithPortfoliosUseCase
  ) {
    super()
  }

  async fetchById(request: Request, { params }: { params: AccountParams }) {
    const { id: organizationId, ledgerId, accountId } = await params

    const account = await this.fetchAccountByIdUseCase.execute(
      organizationId,
      ledgerId,
      accountId!
    )

    return NextResponse.json(account)
  }

  async fetchAll(request: Request, { params }: { params: AccountParams }) {
    const { searchParams } = new URL(request.url)
    const { id: organizationId, ledgerId } = params
    const alias = searchParams.get('alias') ?? undefined
    const limit = Number(searchParams.get('limit')) || 10
    const page = Number(searchParams.get('page')) || 1

    const accounts = await this.fetchAllAccountsUseCase.execute(
      organizationId,
      ledgerId,
      {
        alias,
        limit,
        page
      }
    )

    return NextResponse.json(accounts)
  }

  async create(request: Request, { params }: { params: AccountParams }) {
    const body = await request.json()
    const { id: organizationId, ledgerId } = await params

    const account = await this.createAccountUseCase.execute(
      organizationId,
      ledgerId,
      body
    )

    return NextResponse.json(account)
  }

  async update(request: Request, { params }: { params: AccountParams }) {
    const body = await request.json()
    const { id: organizationId, ledgerId, accountId } = await params

    const accountUpdated = await this.updateAccountUseCase.execute(
      organizationId,
      ledgerId,
      accountId!,
      body
    )

    return NextResponse.json(accountUpdated)
  }

  async delete(request: Request, { params }: { params: AccountParams }) {
    const { id: organizationId, ledgerId, accountId } = await params

    await this.deleteAccountUseCase.execute(
      organizationId,
      ledgerId,
      accountId!
    )

    return NextResponse.json({}, { status: 200 })
  }

  async fetchWithPortfolios(
    request: Request,
    { params }: { params: AccountParams }
  ) {
    const { searchParams } = new URL(request.url)
    const limit = Number(searchParams.get('limit')) || 100
    const page = Number(searchParams.get('page')) || 1
    const alias = searchParams.get('alias') ?? undefined
    const { id: organizationId, ledgerId } = await params

    const accountsWithPortfolios =
      await this.fetchAccountsWithPortfoliosUseCase.execute({
        organizationId,
        ledgerId,
        limit,
        page,
        alias
      })

    return NextResponse.json(accountsWithPortfolios)
  }
}
