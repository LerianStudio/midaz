import { MidazConfigDto } from '@/core/application/dto/midaz-config-dto'
import { getFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

interface UseMidazConfigParams {
  organization: string
  ledger: string
}

export function useMidazConfig() {
  return useQuery<MidazConfigDto>({
    queryKey: ['midaz-config'],
    queryFn: getFetcher(
      `/api/midaz/config`
    ),
    enabled: true,
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000
  })
}
