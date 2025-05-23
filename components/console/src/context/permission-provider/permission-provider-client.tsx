'use client'

import React from 'react'
import { validatePermissions } from './validate-permissions'

export type Permissions = Record<string, string[]>

type PermissionContextProps = {
  permissions: Permissions
  validate: (resource: string, action: string) => boolean
}

const PermissionContext = React.createContext<PermissionContextProps>(
  {} as PermissionContextProps
)

export const usePermissions = () => {
  const context = React.useContext(PermissionContext)

  if (!context) {
    throw new Error('usePermissions must be used within a PermissionProvider')
  }

  return context
}

type PermissionProviderClientProps = React.PropsWithChildren & {
  permissions: Permissions
  wildcard?: string
}

export const PermissionProviderClient = ({
  permissions: permissionsProps,
  wildcard = '*',
  children
}: PermissionProviderClientProps) => {
  const [permissions] = React.useState(permissionsProps)

  const validate = (resource: string, action: string) => {
    const hasPermission = validatePermissions(
      permissions,
      resource,
      action,
      wildcard
    )
    return hasPermission
  }

  return (
    <PermissionContext.Provider value={{ permissions, validate }}>
      {children}
    </PermissionContext.Provider>
  )
}
