import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  CreateLedger,
  CreateLedgerUseCase
} from '@/core/application/use-cases/ledgers/create-ledger-use-case'
import { NextResponse } from 'next/server'
import { getController } from '@/lib/http/server'
import { LedgerController } from '@/core/application/controllers/ledger-controller'

const createLedgerUseCases = container.get<CreateLedger>(CreateLedgerUseCase)

export const GET = getController(LedgerController, (c) => c.fetchAll)

export async function POST(
  request: Request,
  props: { params: Promise<{ id: string }> }
) {
  const params = await props.params
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
