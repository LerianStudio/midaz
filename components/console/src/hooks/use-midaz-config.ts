import { useMidazConfig as useClient } from '@/client/midaz-config'
import { useOrganization } from '@lerianstudio/console-layout'

export const useMidazConfig = () => {
  const { currentOrganization, currentLedger } = useOrganization()
  const { data } = useClient({ organization: currentOrganization.id!, ledger: currentLedger.id })

  return {
    isAccountTypeValidationEnabled: data?.isConfigEnabled && data?.isLedgerAllowed
  }
}