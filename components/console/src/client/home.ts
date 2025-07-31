import { HomeMetricsDto } from '@/core/application/dto/home-metrics-dto'
import { getFetcher, getPaginatedFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

export type UseHomeMetricsProps = {
  organizationId?: string
  ledgerId?: string
}

export function useHomeMetrics({
  organizationId,
  ledgerId
}: UseHomeMetricsProps) {
  return useQuery<HomeMetricsDto>({
    queryKey: ['homeMetrics', organizationId, ledgerId],
    queryFn: getPaginatedFetcher('/api/home/metrics', {
      organizationId,
      ledgerId
    })
  })
}
