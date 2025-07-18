import {
  OrganizationDto,
  OrganizationSearchParamDto
} from '@/core/application/dto/organization-dto'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import {
  deleteFetcher,
  getFetcher,
  getPaginatedFetcher,
  patchFetcher,
  postFetcher
} from '@/lib/fetcher'
import {
  useMutation,
  UseMutationOptions,
  useQuery,
  useQueryClient
} from '@tanstack/react-query'

export type UseListOrganizationsProps = {
  query?: OrganizationSearchParamDto
}

export const useListOrganizations = ({
  query,
  ...options
}: UseListOrganizationsProps) => {
  return useQuery<PaginationDto<OrganizationDto>>({
    queryKey: ['organizations', Object.values(query ?? {})],
    queryFn: getPaginatedFetcher(`/api/organizations`, query),
    ...options
  })
}

export type UseGetOrganizationProps = {
  organizationId: string
  onError?: (error: any) => void
}

export const useGetOrganization = ({
  organizationId,
  ...options
}: UseGetOrganizationProps) => {
  return useQuery({
    queryKey: ['organizations', organizationId],
    queryFn: getFetcher(`/api/organizations/${organizationId}`),
    ...options
  })
}

export const useCreateOrganization = ({
  onSuccess,
  ...options
}: UseMutationOptions<OrganizationDto, Error, any>) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['organizations'],
    mutationFn: postFetcher(`/api/organizations`),
    ...options,
    onSuccess: (data: OrganizationDto, ...args) => {
      queryClient.invalidateQueries({
        queryKey: ['organizations']
      })
      onSuccess?.(data, ...args)
    }
  })
}

export const useUpdateOrganization = ({
  organizationId,
  onSuccess,
  ...options
}: UseGetOrganizationProps &
  UseMutationOptions<OrganizationDto, Error, any>) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['organizations'],
    mutationFn: patchFetcher(`/api/organizations/${organizationId}`),
    ...options,
    onSuccess: (data: OrganizationDto, ...args) => {
      queryClient.invalidateQueries({
        queryKey: ['organizations']
      })
      onSuccess?.(data, ...args)
    }
  })
}

export const useDeleteOrganization = ({
  onSuccess,
  ...options
}: UseMutationOptions<OrganizationDto, Error, unknown>) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['organizations'],
    mutationFn: deleteFetcher(`/api/organizations`),
    ...options,
    onSuccess: (data: OrganizationDto, ...args) => {
      queryClient.invalidateQueries({
        queryKey: ['organizations']
      })
      onSuccess?.(data, ...args)
    }
  })
}
