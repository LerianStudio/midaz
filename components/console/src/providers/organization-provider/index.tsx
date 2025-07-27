'use client'

import {
  createContext,
  PropsWithChildren,
  useContext,
  useEffect,
  useState
} from 'react'
import { OrganizationDto } from '@/core/application/dto/organization-dto'
import { usePathname, useRouter } from 'next/navigation'
import { useListLedgers } from '@/client/ledgers'
import { useDefaultOrg } from './use-default-org'
import { useDefaultLedger } from './use-default-ledger'
import { LedgerDto } from '@/core/application/dto/ledger-dto'
import { useListOrganizations } from '@/client/organizations'

type OrganizationContextProps = {
  currentOrganization: OrganizationDto
  setOrganization: (organization: OrganizationDto) => void
  currentLedger: LedgerDto
  setLedger: (ledger: LedgerDto) => void
}

const OrganizationContext = createContext<OrganizationContextProps>(
  {} as OrganizationContextProps
)

export const useOrganization = () => useContext(OrganizationContext)

export const OrganizationProvider = ({ children }: PropsWithChildren) => {
  const router = useRouter()
  const pathname = usePathname()
  const [current, setCurrent] = useState<OrganizationDto>({} as OrganizationDto)

  const { data: organizations, isPending: loadingOrganizations } =
    useListOrganizations({
      page: 1,
      limit: 100
    })

  const [currentLedger, setCurrentLedger] = useState<LedgerDto>({} as LedgerDto)
  const { data: ledgers, isPending: loadingLedgers } = useListLedgers({
    organizationId: current?.id!,
    limit: 100
  })

  useEffect(() => {
    if (loadingOrganizations || !organizations?.items) {
      return
    }

    if (pathname.includes('/onboarding')) {
      return
    }

    if (organizations.items.length === 0) {
      router.replace('/onboarding')
    }

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

    if (loadingLedgers || !ledgers?.items) {
      return
    }

    if (pathname.includes('/ledgers')) {
      return
    }

    if (!redirectablePaths.some((path) => pathname.includes(path))) {
      return
    }

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
