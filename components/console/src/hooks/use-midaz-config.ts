import { useMidazConfig as useClient } from '@/client/midaz-config'

interface UseMidazConfigParams {
  organization: string
  ledger: string
}

export const useMidazConfig = ({ organization, ledger }: UseMidazConfigParams) => {
  const { data } = useClient({ organization, ledger })

  return {
    isAccountTypeValidationEnabled: data?.isConfigEnabled && data?.isLedgerAllowed
  }
}