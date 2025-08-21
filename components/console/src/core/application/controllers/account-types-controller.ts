import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { NextResponse } from 'next/server'
import { FetchAccountTypesByIdUseCase } from '../use-cases/account-types/fetch-account-types-use-case'
import { FetchAllAccountTypesUseCase } from '../use-cases/account-types/fetch-all-account-types-use-case'
import { CreateAccountTypesUseCase } from '../use-cases/account-types/create-account-types-use-case'
import { DeleteAccountTypesUseCase } from '../use-cases/account-types/delete-account-types-use-case'
import { UpdateAccountTypesUseCase } from '../use-cases/account-types/update-account-types-use-case'
import { BaseController } from '@/lib/http/server/base-controller'

type AccountTypesParams = {
  id: string
  ledgerId: string
  accountTypeId?: string
}

@LoggerInterceptor()
@Controller()
export class AccountTypesController extends BaseController {
  constructor(
    @inject(FetchAllAccountTypesUseCase)
    private readonly fetchAllAccountTypesUseCase: FetchAllAccountTypesUseCase,
    @inject(FetchAccountTypesByIdUseCase)
    private readonly fetchAccountTypesByIdUseCase: FetchAccountTypesByIdUseCase,
    @inject(CreateAccountTypesUseCase)
    private readonly createAccountTypesUseCase: CreateAccountTypesUseCase,
    @inject(UpdateAccountTypesUseCase)
    private readonly updateAccountTypesUseCase: UpdateAccountTypesUseCase,
    @inject(DeleteAccountTypesUseCase)
    private readonly deleteAccountTypesUseCase: DeleteAccountTypesUseCase
  ) {
    super()
  }

  async fetchById(
    request: Request,
    { params }: { params: AccountTypesParams }
  ) {
    const { id: organizationId, ledgerId, accountTypeId } = await params

    const accountType = await this.fetchAccountTypesByIdUseCase.execute(
      organizationId,
      ledgerId,
      accountTypeId!
    )

    return NextResponse.json(accountType)
  }

  async fetchAll(request: Request, { params }: { params: AccountTypesParams }) {
    const { searchParams } = new URL(request.url)
    const { id: organizationId, ledgerId } = await params
    const name = searchParams.get('name') ?? undefined
    const keyValue = searchParams.get('keyValue') ?? undefined
    const limit = Number(searchParams.get('limit')) || 10
    const page = Number(searchParams.get('page')) || 1

    const accountTypes = await this.fetchAllAccountTypesUseCase.execute(
      organizationId,
      ledgerId,
      {
        name,
        keyValue,
        limit,
        page
      }
    )

    return NextResponse.json(accountTypes)
  }

  async create(request: Request, { params }: { params: AccountTypesParams }) {
    const body = await request.json()
    const { id: organizationId, ledgerId } = await params

    const accountType = await this.createAccountTypesUseCase.execute(
      organizationId,
      ledgerId,
      body
    )

    return NextResponse.json(accountType)
  }

  async update(request: Request, { params }: { params: AccountTypesParams }) {
    const body = await request.json()
    const { id: organizationId, ledgerId, accountTypeId } = await params

    const { metadata, ...restBody } = body
    const updateData = metadata === null ? restBody : body

    const accountTypeUpdated = await this.updateAccountTypesUseCase.execute(
      organizationId,
      ledgerId,
      accountTypeId!,
      updateData
    )

    return NextResponse.json(accountTypeUpdated)
  }

  async delete(request: Request, { params }: { params: AccountTypesParams }) {
    const { id: organizationId, ledgerId, accountTypeId } = await params

    await this.deleteAccountTypesUseCase.execute(
      organizationId,
      ledgerId,
      accountTypeId!
    )

    return NextResponse.json({}, { status: 200 })
  }
}
