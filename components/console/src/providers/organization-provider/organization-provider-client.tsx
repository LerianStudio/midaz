'use client'

import {
  createContext,
  PropsWithChildren,
  useContext,
  useEffect,
  useState
} from 'react'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { usePathname, useRouter } from 'next/navigation'
import { useListLedgers } from '@/client/ledgers'
import { LedgerType } from '@/types/ledgers-type'
import { useDefaultOrg } from './use-default-org'
import { useDefaultLedger } from './use-default-ledger'

type OrganizationContextProps = {
  currentOrganization: OrganizationEntity
  setOrganization: (organization: OrganizationEntity) => void
  currentLedger: LedgerType
  setLedger: (ledger: LedgerType) => void
}

const OrganizationContext = createContext<OrganizationContextProps>(
  {} as OrganizationContextProps
)

export const useOrganization = () => useContext(OrganizationContext)

export type OrganizationProviderClientProps = PropsWithChildren & {
  organizations: OrganizationEntity[]
}

export const OrganizationProviderClient = ({
  organizations: organizationsProp,
  children
}: OrganizationProviderClientProps) => {
  const router = useRouter()
  const pathname = usePathname()
  const [current, setCurrent] = useState<OrganizationEntity>(
    {} as OrganizationEntity
  )
  const [organizations, setOrganizations] = useState<OrganizationEntity[]>(
    organizationsProp ?? []
  )

  const [currentLedger, setCurrentLedger] = useState<LedgerType>(
    {} as LedgerType
  )
  const { data: ledgers } = useListLedgers({
    organizationId: current?.id!,
    limit: 100
  })

  useEffect(() => {
    // Do nothing if the user is already at the onboarding
    if (pathname.includes('/onboarding')) {
      return
    }

    // Redirect user to onboarding if it has no organizations
    if (organizations.length === 0) {
      router.replace('/onboarding')
    }

    // Redirect user to ledger onboarding if it has only one organization and no ledgers
    if (organizations.length === 1 && current?.metadata?.onboarding === true) {
      router.replace('/onboarding/ledger')
    }
  }, [current?.id, organizations.length])

  useDefaultOrg({
    organizations,
    current,
    setCurrent
  })

  useDefaultLedger({
    current,
    ledgers: ledgers?.items,
    currentLedger,
    setCurrentLedger
  })

  return (
    <OrganizationContext.Provider
      value={{
        currentOrganization: current,
        setOrganization: setCurrent,
        currentLedger: currentLedger,
        setLedger: setCurrentLedger
      }}
    >
      {children}
    </OrganizationContext.Provider>
  )
}
