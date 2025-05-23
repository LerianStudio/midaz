import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../../../utils/api-error-handler'
import {
  CompleteOnboarding,
  CompleteOnboardingUseCase
} from '@/core/application/use-cases/onboarding/complete-onboarding-use-case'

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'completeOnboarding',
      method: 'POST'
    })
  ],
  async (request: Request, { params }: { params: { id: string } }) => {
    try {
      const completeOnboardingUseCase: CompleteOnboarding =
        container.get<CompleteOnboarding>(CompleteOnboardingUseCase)
      const body = await request.json()

      const ledger = await completeOnboardingUseCase.execute(params.id, body)

      return NextResponse.json(ledger)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
