'use server'

import {
  AuthPermission,
  AuthPermissionUseCase
} from '@/core/application/use-cases/auth/auth-permission-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'
import { getServerSession } from 'next-auth'
import { PermissionProviderClient, Permissions } from './permission-provider-client'
import { serverFetcher } from '@/lib/fetcher'

const authPermissionUseCase = container.get<AuthPermission>(
  AuthPermissionUseCase
)

// Default permissions when auth is disabled (full access)
const DEFAULT_PERMISSIONS: Permissions = {
  '*': ['*']
}

export const PermissionProvider = async ({
  children
}: React.PropsWithChildren) => {
  let permissions: Permissions = DEFAULT_PERMISSIONS

  if (process.env.PLUGIN_AUTH_ENABLED === 'true') {
    const session = await getServerSession(nextAuthOptions)

    if (session) {
      const authPermissions = await serverFetcher(
        async () => await authPermissionUseCase.execute()
      )
      permissions = authPermissions || DEFAULT_PERMISSIONS
    }
  }

  return (
    <PermissionProviderClient permissions={permissions}>
      {children}
    </PermissionProviderClient>
  )
}
