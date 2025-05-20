import { deleteFetcher, getFetcher, postFetcher } from '@/lib/fetcher'
import {
  useMutation,
  UseMutationOptions,
  useQuery,
  useQueryClient
} from '@tanstack/react-query'
import {
  ApplicationResponseDto,
  CreateApplicationDto
} from '@/core/application/dto/application-dto'

export const useApplications = ({ ...options } = {}) => {
  return useQuery<ApplicationResponseDto[]>({
    queryKey: ['applications'],
    queryFn: getFetcher('/api/identity/applications'),
    ...options
  })
}

export const useApplicationById = ({ id, ...options }: { id: string }) => {
  return useQuery<ApplicationResponseDto>({
    queryKey: ['applications', id],
    queryFn: getFetcher(`/api/identity/applications/${id}`),
    enabled: !!id,
    ...options
  })
}

export const useCreateApplication = ({
  ...options
}: UseMutationOptions<any, any, CreateApplicationDto> = {}) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['applications', 'create'],
    mutationFn: postFetcher('/api/identity/applications'),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['applications']
      })
      options.onSuccess?.(...args)
    },
    ...options
  })
}

export const useDeleteApplication = ({
  ...options
}: UseMutationOptions<any, any, any> = {}) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['applications', 'delete'],
    mutationFn: deleteFetcher('/api/identity/applications'),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['applications']
      })
      options.onSuccess?.(...args)
    },
    ...options
  })
}
