'use client'

import { OrganizationResponseDto } from '@/core/application/dto/organization-dto'
import { getStorage } from '@/lib/storage'
import { isNil } from 'lodash'
import { useEffect, useState } from 'react'

type UseDefaultOrgProps = {
  organizations?: OrganizationResponseDto[]
  current: OrganizationResponseDto
  setCurrent: (organization: OrganizationResponseDto) => void
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
    // We should never set a default if no organization is found
    if (isNil(organizations)) {
      return
    }

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
  }, [organizations])

  useEffect(() => {
    // Update storage according to the current organization
    if (current?.id) {
      save(current.id)
    }
  }, [current?.id])
}
