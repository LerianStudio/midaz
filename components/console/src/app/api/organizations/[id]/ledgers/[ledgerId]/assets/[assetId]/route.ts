import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  DeleteAsset,
  DeleteAssetUseCase
} from '@/core/application/use-cases/assets/delete-asset-use-case'
import {
  FetchAssetById,
  FetchAssetByIdUseCase
} from '@/core/application/use-cases/assets/fetch-asset-by-id-use-case'
import {
  UpdateAsset,
  UpdateAssetUseCase
} from '@/core/application/use-cases/assets/update-asset-use-case'
import { NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAssetById',
      method: 'GET'
    })
  ],
  async (
    _,
    { params }: { params: { id: string; ledgerId: string; assetId: string } }
  ) => {
    try {
      const fetchAssetByIdUseCase: FetchAssetById =
        container.get<FetchAssetById>(FetchAssetByIdUseCase)
      const { id, ledgerId, assetId } = await params

      const assets = await fetchAssetByIdUseCase.execute(id, ledgerId, assetId)

      return NextResponse.json(assets)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const PATCH = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'updateAsset',
      method: 'PATCH'
    })
  ],
  async (
    request: Request,
    { params }: { params: { id: string; ledgerId: string; assetId: string } }
  ) => {
    try {
      const updateAssetUseCase = container.get<UpdateAsset>(UpdateAssetUseCase)

      const { id, ledgerId, assetId } = await params
      const body = await request.json()

      const assetUpdated = await updateAssetUseCase.execute(
        id,
        ledgerId,
        assetId,
        body
      )

      return NextResponse.json(assetUpdated)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const DELETE = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'deleteAsset',
      method: 'DELETE'
    })
  ],
  async (
    request: Request,
    { params }: { params: { id: string; ledgerId: string; assetId: string } }
  ) => {
    try {
      const deleteAssetUseCase = container.get<DeleteAsset>(DeleteAssetUseCase)

      const { id, ledgerId, assetId } = await params

      await deleteAssetUseCase.execute(id, ledgerId, assetId)

      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
