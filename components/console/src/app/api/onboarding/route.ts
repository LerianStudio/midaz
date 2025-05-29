import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../utils/api-error-handler'
import {
  CreateOnboardingOrganization,
  CreateOnboardingOrganizationUseCase
} from '@/core/application/use-cases/onboarding/create-onboarding-organization-use-case'

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createOnboardingOrganization',
      method: 'POST'
    })
  ],
  async (request: Request) => {
    try {
      const createOnboardingOrganization: CreateOnboardingOrganization =
        await container.getAsync<CreateOnboardingOrganization>(
          CreateOnboardingOrganizationUseCase
        )

      const body = await request.json()
      const organization = await createOnboardingOrganization.execute(body)

      return NextResponse.json(organization)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
