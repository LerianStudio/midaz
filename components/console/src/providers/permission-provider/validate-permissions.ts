import type { Permissions } from './permission-provider-client'

/**
 * Method to validate if a resource and action is part of a set of permissions.
 * It uses wildcard logic.
 * Should only be used inside the PermissionProvider.
 *
 * @param permissions Permissions record
 * @param resource Current resource to be checked
 * @param action Current action to be checked
 * @param wildcard Wildcard, default to '*'
 * @returns boolean
 */
export const validatePermissions = (
  permissions: Permissions,
  resource: string,
  action: string,
  wildcard = '*'
) => {
  if (permissions.hasOwnProperty(wildcard)) {
    return true
  }

  if (!permissions[resource]) {
    return false
  }

  if (permissions[resource].includes(wildcard)) {
    return true
  }

  return permissions[resource].includes(action)
}
