'use client'

import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { getStorage } from '@/lib/storage'
import { useEffect, useState } from 'react'

type UseDefaultOrgProps = {
  organizations: OrganizationEntity[]
  current: OrganizationEntity
  setCurrent: (organization: OrganizationEntity) => void
}

const storageKey = 'defaultOrg'

export function useDefaultOrg({
  organizations,
  current,
  setCurrent
}: UseDefaultOrgProps) {
  const [defaultOrg, setDefaultOrg] = useState<string | null>(
    getStorage(storageKey, null)
  )

  const save = (id: string) => {
    localStorage.setItem(storageKey, id)
    setDefaultOrg(id)
  }

  // Initialize a current organization
  useEffect(() => {
    // Check if there is a default organization saved onto local storage
    if (defaultOrg) {
      // Search for the organization with the id
      const org = organizations.find(({ id }) => defaultOrg === id)

      // If the organization is found, set it as the current organization
      if (org) {
        setCurrent(org)
        return
      }
    }

    // If there is no default organization saved or the organization is not found
    if (organizations.length > 0) {
      // Set the first organization as the current one
      setCurrent(organizations[0])
    }
  }, [])

  useEffect(() => {
    // Update storage according to the current organization
    if (current?.id) {
      save(current.id)
    }
  }, [current?.id])
}
