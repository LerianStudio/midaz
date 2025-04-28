import { AccountResponseDto } from '@/core/application/dto/account-dto'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import {
  deleteFetcher,
  getFetcher,
  getPaginatedFetcher,
  patchFetcher,
  postFetcher
} from '@/lib/fetcher'
import { PaginationRequest } from '@/types/pagination-request-type'
import {
  useMutation,
  UseMutationOptions,
  useQuery
} from '@tanstack/react-query'

type UseListAccountsProps = {
  organizationId: string
  ledgerId: string
}

export const useListAccounts = ({
  organizationId,
  ledgerId,
  ...options
}: UseListAccountsProps) => {
  return useQuery<PaginationDto<AccountEntity>>({
    queryKey: [organizationId, ledgerId, 'accounts'],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/accounts`
    ),
    enabled: !!organizationId && !!ledgerId,
    ...options
  })
}

type UseAccountsWithPortfoliosProps = PaginationRequest & {
  organizationId: string
  ledgerId: string
}

export const useAccountsWithPortfolios = ({
  organizationId,
  ledgerId,
  page,
  limit,
  ...options
}: UseAccountsWithPortfoliosProps) => {
  return useQuery<PaginationDto<AccountResponseDto>>({
    queryKey: [
      organizationId,
      ledgerId,
      'accounts-with-portfolios',
      page,
      limit
    ],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/accounts-portfolios`,
      { page, limit }
    ),
    enabled: !!organizationId && !!ledgerId,
    ...options
  })
}

type UseDeleteAccountProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
  accountId: string
}

export const useDeleteAccount = ({
  organizationId,
  ledgerId,
  accountId,
  ...options
}: UseDeleteAccountProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId, accountId],
    mutationFn: deleteFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/accounts`
    ),
    ...options
  })
}

type UseCreateAccountProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

export const useCreateAccount = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreateAccountProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId],
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/accounts`
    ),
    ...options
  })
}

type UseUpdateAccountProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
  accountId: string
}

export const useUpdateAccount = ({
  organizationId,
  ledgerId,
  accountId,
  ...options
}: UseUpdateAccountProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId, accountId],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`
    ),
    ...options
  })
}
