import { MidazConfigValidationDto } from '@/core/application/dto/midaz-config-dto'
import { getFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

interface UseMidazConfigParams {
  organization: string
  ledger: string
}

export function useMidazConfig({ organization, ledger }: UseMidazConfigParams) {
  return useQuery<MidazConfigValidationDto>({
    queryKey: ['midaz-config', organization, ledger],
    queryFn: getFetcher(`/api/midaz/config?organization=${encodeURIComponent(organization)}&ledger=${encodeURIComponent(ledger)}`),
    enabled: Boolean(organization && ledger)
  })
}