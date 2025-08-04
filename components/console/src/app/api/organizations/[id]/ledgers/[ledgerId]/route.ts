import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  DeleteLedger,
  DeleteLedgerUseCase
} from '@/core/application/use-cases/ledgers/delete-ledger-use-case'
import {
  FetchLedgerById,
  FetchLedgerByIdUseCase
} from '@/core/application/use-cases/ledgers/fetch-ledger-by-id-use-case'
import {
  UpdateLedger,
  UpdateLedgerUseCase
} from '@/core/application/use-cases/ledgers/update-ledger-use-case'
import { NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchLedgerById',
      method: 'GET'
    })
  ],
  async (_, { params }: { params: { id: string; ledgerId: string } }) => {
    try {
      const fetchLedgerByIdUseCase: FetchLedgerById =
        container.get<FetchLedgerById>(FetchLedgerByIdUseCase)
      const { id: organizationId, ledgerId } = await params

      const ledgers = await fetchLedgerByIdUseCase.execute(
        organizationId,
        ledgerId
      )

      return NextResponse.json(ledgers)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const PATCH = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'updateLedger',
      method: 'PATCH'
    })
  ],
  async (
    request: Request,
    { params }: { params: { id: string; ledgerId: string } }
  ) => {
    try {
      const updateLedgerUseCase =
        container.get<UpdateLedger>(UpdateLedgerUseCase)

      const body = await request.json()
      const { id: organizationId, ledgerId } = await params

      const ledgerUpdated = await updateLedgerUseCase.execute(
        organizationId,
        ledgerId,
        body
      )

      return NextResponse.json({ ledgerUpdated })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const DELETE = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'deleteLedger',
      method: 'DELETE'
    })
  ],
  async (_, { params }: { params: { id: string; ledgerId: string } }) => {
    try {
      const deleteLedgerUseCase =
        container.get<DeleteLedger>(DeleteLedgerUseCase)

      const { id: organizationId, ledgerId } = await params

      await deleteLedgerUseCase.execute(organizationId, ledgerId)

      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
