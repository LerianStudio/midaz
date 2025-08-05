import { PluginManifestDto } from '@/core/application/dto/plugin-manifest-dto'
import { getFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

export const useGetPluginMenus = ({ ...options } = {}) => {
  return useQuery<PluginManifestDto[]>({
    queryKey: ['plugin-menus'],
    queryFn: getFetcher('/api/plugin/menu'),
    staleTime: 1000 * 60 * 60,
    ...options
  })
}
