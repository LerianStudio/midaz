import { MidazConfigDto } from '@/core/application/dto/midaz-config-dto'
import { getFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

interface UseMidazConfigParams {
  organization: string
  ledger: string
}

export function useMidazConfig({ organization, ledger }: UseMidazConfigParams) {
  return useQuery<MidazConfigDto>({
    queryKey: ['midaz-config', organization],
    queryFn: getFetcher(`/api/midaz/config?organization=${encodeURIComponent(organization)}&ledger=${encodeURIComponent(ledger)}`),
    enabled: Boolean(organization && ledger),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000, 
  })
}