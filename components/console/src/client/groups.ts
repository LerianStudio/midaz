import { getFetcher } from '@/lib/fetcher'
import { keepPreviousData, useQuery } from '@tanstack/react-query'

export const useListGroups = ({ ...options }) => {
  return useQuery<any>({
    queryKey: ['groups'],
    queryFn: getFetcher(`/api/identity/groups`),
    placeholderData: keepPreviousData,
    ...options
  })
}
