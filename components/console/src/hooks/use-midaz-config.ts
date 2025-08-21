import { useOrganization } from '@lerianstudio/console-layout'
import { useMemo } from 'react'
import { useMidazConfig as useClientMidazConfig } from '@/client/midaz-config'

export const useMidazConfig = () => {
  const { currentOrganization, currentLedger } = useOrganization()

  const { data: config, isLoading, error, refetch } = useClientMidazConfig({
    organization: currentOrganization?.id || '',
    ledger: currentLedger?.id || ''
  })

  const isAccountTypeValidationEnabled = useMemo(() => {
    if (!config?.isConfigEnabled) return false
    
    const isLedgerAllowed = config.config.some((org: any) => 
      org.organization === currentOrganization?.id && 
      org.ledgers.includes(currentLedger?.id || '')
    )

    return config.isConfigEnabled && isLedgerAllowed
  }, [config, currentOrganization?.id, currentLedger?.id])

  return {
    isAccountTypeValidationEnabled,
    isLoading,
    error: error?.message || null,
    config,
    retry: refetch
  }
}