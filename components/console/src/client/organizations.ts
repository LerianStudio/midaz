import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import useCustomToast from '@/hooks/use-custom-toast'
import {
  deleteFetcher,
  getFetcher,
  patchFetcher,
  postFetcher
} from '@/lib/fetcher'
import {
  useMutation,
  UseMutationOptions,
  useQuery
} from '@tanstack/react-query'

export const useListOrganizations = ({ ...options }) => {
  return useQuery<PaginationDto<OrganizationEntity>>({
    queryKey: ['organizations'],
    queryFn: getFetcher(`/api/organizations`),
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

export const useCreateOrganization = ({ ...options }) => {
  const { showError } = useCustomToast()

  return useMutation({
    mutationKey: ['organizations'],
    mutationFn: postFetcher(`/api/organizations`),
    ...options,
    onError: (error) => {
      showError(error.message)
      options.onError?.(error)
    }
  })
}

export const useUpdateOrganization = ({
  organizationId,
  ...options
}: UseGetOrganizationProps & UseMutationOptions<any, any, any>) => {
  const { showError } = useCustomToast()

  return useMutation({
    mutationKey: ['organizations'],
    mutationFn: patchFetcher(`/api/organizations/${organizationId}`),
    ...options,
    onError: (error) => {
      showError(error.message)
      options.onError?.(error)
    }
  })
}

export const useDeleteOrganization = ({ ...options }) => {
  return useMutation({
    mutationKey: ['organizations'],
    mutationFn: deleteFetcher(`/api/organizations`),
    ...options
  })
}
