import { usePermissions } from '@/providers/permission-provider/permission-provider-client'

/**
 * Hook to determine form permissions based on user's resource permissions
 * @param resource The resource name (e.g., 'assets', 'accounts')
 * @returns Object containing permission states for the form
 */

export function useFormPermissions(resource: string) {
  const isAuthEnabled = process.env.NEXT_PUBLIC_MIDAZ_AUTH_ENABLED === 'true'

  if (!isAuthEnabled) {
    return {
      hasReadPermission: true,
      hasWritePermission: true,
      isReadOnly: false
    }
  }

  const { validate } = usePermissions()

  const hasReadPermission = validate(resource, 'get')
  const hasWritePermission =
    validate(resource, 'patch') || validate(resource, 'post')

  const isReadOnly = hasReadPermission && !hasWritePermission

  return {
    hasReadPermission,
    hasWritePermission,
    isReadOnly
  }
}
