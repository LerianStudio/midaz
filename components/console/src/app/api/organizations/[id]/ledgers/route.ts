import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  CreateLedger,
  CreateLedgerUseCase
} from '@/core/application/use-cases/ledgers/create-ledger-use-case'
import {
  FetchAllLedgers,
  FetchAllLedgersUseCase
} from '@/core/application/use-cases/ledgers/fetch-all-ledgers-use-case'
import { NextResponse } from 'next/server'

const fetchAllLedgersUseCases = container.get<FetchAllLedgers>(
  FetchAllLedgersUseCase
)
const createLedgerUseCases = container.get<CreateLedger>(CreateLedgerUseCase)

export async function GET(
  request: Request,
  { params }: { params: { id: string } }
) {
  try {
    const { searchParams } = new URL(request.url)
    const limit = Number(searchParams.get('limit')) || 10
    const page = Number(searchParams.get('page')) || 1
    const organizationId = params.id

    const ledgers = await fetchAllLedgersUseCases.execute(
      organizationId,
      limit,
      page
    )

    return NextResponse.json(ledgers)
  } catch (error: any) {
    const { message, status } = await apiErrorHandler(error)

    return NextResponse.json({ message }, { status })
  }
}

export async function POST(
  request: Request,
  { params }: { params: { id: string } }
) {
  try {
    const organizationId = params.id
    const body = await request.json()

    const ledger = await createLedgerUseCases.execute(organizationId, body)
    return NextResponse.json({ ledger }, { status: 201 })
  } catch (error: any) {
    const { message, status } = await apiErrorHandler(error)

    return NextResponse.json({ message }, { status })
  }
}
