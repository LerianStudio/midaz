import { MidazInfoDto } from '@/core/application/dto/midaz-info-dto'
import { getFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

export function useGetMidazInfo() {
  return useQuery<MidazInfoDto>({
    queryKey: ['midaz-info'],
    queryFn: getFetcher('/api/midaz/info')
  })
}
