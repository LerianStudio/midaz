import { VersionDto } from '@/core/application/dto/version-dto'
import { getFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

export function useGetVersion() {
  return useQuery<VersionDto>({
    queryKey: ['version'],
    queryFn: getFetcher('/api/version')
  })
}
