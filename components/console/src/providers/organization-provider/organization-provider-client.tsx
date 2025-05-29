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
import { useListOrganizations } from '@/client/organizations'

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

  const { data: organizations, isPending: loadingOrganizations } =
    useListOrganizations({
      page: 1,
      limit: 100,
      initialData: {
        items: organizationsProp
      }
    })

  const [currentLedger, setCurrentLedger] = useState<LedgerResponseDto>(
    {} as LedgerResponseDto
  )
  const { data: ledgers, isPending: loadingLedgers } = useListLedgers({
    organizationId: current?.id!,
    limit: 100
  })

  useEffect(() => {
    // Wait the request to finish
    if (loadingOrganizations || !organizations?.items) {
      return
    }

    // Do nothing if the user is already at the onboarding
    if (pathname.includes('/onboarding')) {
      return
    }

    // Redirect user to onboarding if it has no organizations
    if (organizations.items.length === 0) {
      router.replace('/onboarding')
    }

    // Redirect user to ledger onboarding if it has only one organization and no ledgers
    if (
      organizations.items.length === 1 &&
      current?.metadata?.onboarding === true
    ) {
      router.replace('/onboarding/ledger')
    }
  }, [loadingOrganizations, current?.id, organizations?.items])

  useEffect(() => {
    const redirectablePaths = [
      '/assets',
      '/accounts',
      '/segments',
      '/portfolios',
      '/transactions'
    ]

    // Do nothing if the request is still ongoing
    if (loadingLedgers || !ledgers?.items) {
      return
    }

    // If the user is already on the ledgers page, do nothing
    if (pathname.includes('/ledgers')) {
      return
    }

    // If the user is not on a redirectable path, do nothing
    if (!redirectablePaths.some((path) => pathname.includes(path))) {
      return
    }

    // If the user has no ledgers, redirect to the ledgers page
    if (ledgers.items.length === 0) {
      router.replace('/ledgers')
    }
  }, [loadingLedgers, ledgers?.items])

  useDefaultOrg({
    organizations: organizations?.items,
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
