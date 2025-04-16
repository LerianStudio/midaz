import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { PortfolioViewResponseDTO } from '@/core/application/dto/portfolio-view-dto'
import { PortfolioResponseDto } from '@/core/application/dto/portfolios-dto'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import {
  getFetcher,
  postFetcher,
  patchFetcher,
  deleteFetcher,
  getPaginatedFetcher
} from '@/lib/fetcher'
import { PaginationRequest } from '@/types/pagination-request-type'
import {
  useMutation,
  UseMutationOptions,
  useQuery
} from '@tanstack/react-query'

type UseCreatePortfolioProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

type UsePortfoliosWithAccountsProps = PaginationRequest & {
  organizationId: string
  ledgerId: string
}

export const usePortfoliosWithAccounts = ({
  organizationId,
  ledgerId,
  page,
  limit,
  ...options
}: UsePortfoliosWithAccountsProps) => {
  return useQuery<PaginationDto<PortfolioViewResponseDTO>>({
    queryKey: [
      organizationId,
      ledgerId,
      'portfolios-with-accounts',
      page,
      limit
    ],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/portfolios-accounts`,
      { page, limit }
    ),
    ...options
  })
}

export const useCreatePortfolio = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreatePortfolioProps) => {
  return useMutation<any, any, any>({
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/portfolios`
    ),
    ...options
  })
}

type UseListPortfoliosProps = UseCreatePortfolioProps

export const useListPortfolios = ({
  organizationId,
  ledgerId,
  ...options
}: UseListPortfoliosProps) => {
  return useQuery<PaginationDto<PortfolioEntity>>({
    queryKey: [organizationId, ledgerId, 'portfolios'],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/portfolios`
    ),
    ...options
  })
}

type UseUpdatePortfolioProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
  portfolioId: string
}

export const useUpdatePortfolio = ({
  organizationId,
  ledgerId,
  portfolioId,
  ...options
}: UseUpdatePortfolioProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId, portfolioId],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`
    ),
    ...options
  })
}

type UseDeletePortfolioProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

export const useDeletePortfolio = ({
  organizationId,
  ledgerId,
  ...options
}: UseDeletePortfolioProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId],
    mutationFn: deleteFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/portfolios`
    ),
    ...options
  })
}

type UseGetPortfolioProps = {
  organizationId: string
  ledgerId: string
  portfolioId: string
} & UseMutationOptions

export const useGetPortfolio = ({
  organizationId,
  ledgerId,
  portfolioId,
  ...options
}: UseGetPortfolioProps) => {
  return useQuery<PortfolioResponseDto>({
    queryKey: [organizationId, ledgerId, 'portfolio', portfolioId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`
    ),
    ...options
  })
}
