import {
  AccountTypesDto,
  AccountTypesSearchParamDto,
  CreateAccountTypesDto,
  UpdateAccountTypesDto
} from '@/core/application/dto/account-types-dto'
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
  UseQueryOptions
} from '@tanstack/react-query'

type UseListAccountTypesProps = {
  organizationId: string
  ledgerId: string
  query?: AccountTypesSearchParamDto
  enabled?: boolean
} & Omit<
  UseQueryOptions<PaginationDto<AccountTypesDto>>,
  'queryKey' | 'queryFn'
>

export const useListAccountTypes = ({
  organizationId,
  ledgerId,
  query,
  enabled = true,
  ...options
}: UseListAccountTypesProps) => {
  return useQuery<PaginationDto<AccountTypesDto>>({
    queryKey: [
      organizationId,
      ledgerId,
      'account-types',
      ...Object.values(query ?? {})
    ],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/account-types`,
      query
    ),
    enabled: !!organizationId && !!ledgerId && enabled,
    ...options
  })
}

type UseAccountTypeProps = {
  organizationId: string
  ledgerId: string
  accountTypeId: string
  enabled?: boolean
} & Omit<UseQueryOptions<AccountTypesDto>, 'queryKey' | 'queryFn'>

export const useAccountType = ({
  organizationId,
  ledgerId,
  accountTypeId,
  enabled = true,
  ...options
}: UseAccountTypeProps) => {
  return useQuery<AccountTypesDto>({
    queryKey: [organizationId, ledgerId, 'account-types', accountTypeId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/account-types/${accountTypeId}`
    ),
    enabled: !!organizationId && !!ledgerId && !!accountTypeId && enabled,
    ...options
  })
}

type UseCreateAccountTypeProps = {
  organizationId: string
  ledgerId: string
} & UseMutationOptions<AccountTypesDto, any, CreateAccountTypesDto>

export const useCreateAccountType = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreateAccountTypeProps) => {
  return useMutation<AccountTypesDto, any, CreateAccountTypesDto>({
    mutationKey: ['create-account-type', organizationId, ledgerId],
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/account-types`
    ),
    ...options
  })
}

type UseUpdateAccountTypeProps = {
  organizationId: string
  ledgerId: string
  accountTypeId: string
} & UseMutationOptions<AccountTypesDto, any, UpdateAccountTypesDto>

export const useUpdateAccountType = ({
  organizationId,
  ledgerId,
  accountTypeId,
  ...options
}: UseUpdateAccountTypeProps) => {
  return useMutation<AccountTypesDto, any, UpdateAccountTypesDto>({
    mutationKey: [
      'update-account-type',
      organizationId,
      ledgerId,
      accountTypeId
    ],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/account-types/${accountTypeId}`
    ),
    ...options
  })
}

type UseDeleteAccountTypeProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
  accountTypeId: string
}

export const useDeleteAccountType = ({
  organizationId,
  ledgerId,
  accountTypeId,
  ...options
}: UseDeleteAccountTypeProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId, accountTypeId],
    mutationFn: deleteFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/account-types`
    ),
    ...options
  })
}
