'use client'

import { OrganizationDto } from '@/core/application/dto/organization-dto'
import { getStorage } from '@/lib/storage'
import { isNil } from 'lodash'
import { useEffect, useState } from 'react'

type UseDefaultOrgProps = {
  organizations?: OrganizationDto[]
  current: OrganizationDto
  setCurrent: (organization: OrganizationDto) => void
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

  useEffect(() => {
    if (isNil(organizations)) {
      return
    }

    if (defaultOrg) {
      const org = organizations.find(({ id }) => defaultOrg === id)

      if (org) {
        setCurrent(org)
        return
      }
    }

    if (organizations.length > 0) {
      setCurrent(organizations[0])
    }
  }, [organizations])

  useEffect(() => {
    if (current?.id) {
      save(current.id)
    }
  }, [current?.id])
}
