import { cookies } from 'next/headers'

/**
 * Gets the current organization ID from cookies
 * This is a temporary implementation until proper user/org context is available
 */
export async function getOrganizationId(): Promise<string> {
  // Try to get from cookie first
  const cookieStore = cookies()
  const orgId = cookieStore.get('organizationId')?.value

  if (orgId) {
    return orgId
  }

  // Fallback to a default organization ID for testing
  // In production, this should throw an error or redirect to org selection
  return '01956b69-9102-71dd-ba69-0242ac180002'
}
