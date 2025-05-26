'use client'

import {
  createContext,
  PropsWithChildren,
  useContext,
  useEffect,
  useState
} from 'react'
import { OrganizationResponseDto } from '@/core/application/dto/organization-dto'
import { usePathname, useRouter } from 'next/navigation'
import { useListLedgers } from '@/client/ledgers'
import { useDefaultOrg } from './use-default-org'
import { useDefaultLedger } from './use-default-ledger'
import { LedgerResponseDto } from '@/core/application/dto/ledger-dto'

type OrganizationContextProps = {
  currentOrganization: OrganizationResponseDto
  setOrganization: (organization: OrganizationResponseDto) => void
  currentLedger: LedgerResponseDto
  setLedger: (ledger: LedgerResponseDto) => void
}

const OrganizationContext = createContext<OrganizationContextProps>(
  {} as OrganizationContextProps
)

export const useOrganization = () => useContext(OrganizationContext)

export type OrganizationProviderClientProps = PropsWithChildren & {
  organizations: OrganizationResponseDto[]
}

export const OrganizationProviderClient = ({
  organizations: organizationsProp,
  children
}: OrganizationProviderClientProps) => {
  const router = useRouter()
  const pathname = usePathname()
  const [current, setCurrent] = useState<OrganizationResponseDto>(
    {} as OrganizationResponseDto
  )
  const [organizations, setOrganizations] = useState<OrganizationResponseDto[]>(
    organizationsProp ?? []
  )

  const [currentLedger, setCurrentLedger] = useState<LedgerResponseDto>(
    {} as LedgerResponseDto
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
