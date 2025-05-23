import { validatePermissions } from './validate-permissions'
import type { Permissions } from './permission-provider-client'

describe('validatePermissions', () => {
  const wildcard = '*'

  it('should return true if wildcard permission is present', () => {
    const permissions: Permissions = { [wildcard]: [''] }
    expect(validatePermissions(permissions, 'resource', 'action')).toBe(true)
  })

  it('should return false if resource is not present in permissions', () => {
    const permissions: Permissions = {}
    expect(validatePermissions(permissions, 'resource', 'action')).toBe(false)
  })

  it('should return true if resource has wildcard permission', () => {
    const permissions: Permissions = { resource: [wildcard] }
    expect(validatePermissions(permissions, 'resource', 'action')).toBe(true)
  })

  it('should return true if resource has specific action permission', () => {
    const permissions: Permissions = { resource: ['action'] }
    expect(validatePermissions(permissions, 'resource', 'action')).toBe(true)
  })

  it('should return false if resource does not have specific action permission', () => {
    const permissions: Permissions = { resource: ['otherAction'] }
    expect(validatePermissions(permissions, 'resource', 'action')).toBe(false)
  })
})
