import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  CreateAsset,
  CreateAssetUseCase
} from '@/core/application/use-cases/assets/create-asset-use-case'
import {
  FetchAllAssets,
  FetchAllAssetsUseCase
} from '@/core/application/use-cases/assets/fetch-all-assets-use-case'
import { NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createAsset',
      method: 'POST'
    })
  ],
  async (
    request: Request,
    { params }: { params: { id: string; ledgerId: string } }
  ) => {
    try {
      const createAssetUseCase: CreateAsset =
        container.get<CreateAsset>(CreateAssetUseCase)
      const body = await request.json()
      const organizationId = params.id
      const ledgerId = params.ledgerId

      const assetCreated = await createAssetUseCase.execute(
        organizationId,
        ledgerId,
        body
      )

      return NextResponse.json(assetCreated)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllAssets',
      method: 'GET'
    })
  ],
  async (
    request: Request,
    { params }: { params: { id: string; ledgerId: string } }
  ) => {
    try {
      const fetchAllAssetsUseCase: FetchAllAssets =
        container.get<FetchAllAssets>(FetchAllAssetsUseCase)
      const { searchParams } = new URL(request.url)
      const limit = Number(searchParams.get('limit')) || 10
      const page = Number(searchParams.get('page')) || 1
      const organizationId = params.id
      const ledgerId = params.ledgerId

      const assets = await fetchAllAssetsUseCase.execute(
        organizationId,
        ledgerId,
        limit,
        page
      )

      return NextResponse.json(assets)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
