'use server'

import {
  AuthPermission,
  AuthPermissionUseCase
} from '@/core/application/use-cases/auth/auth-permission-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'
import { getServerSession } from 'next-auth'
import { PermissionProviderClient } from './permission-provider-client'
import { serverFetcher } from '@/lib/fetcher'

const authPermissionUseCase = container.get<AuthPermission>(
  AuthPermissionUseCase
)

export const PermissionProvider = async ({
  children
}: React.PropsWithChildren) => {
  const _session = await getServerSession(nextAuthOptions)

  const permissions = await serverFetcher(
    async () => await authPermissionUseCase.execute()
  )

  return (
    <PermissionProviderClient permissions={permissions!}>
      {children}
    </PermissionProviderClient>
  )
}
