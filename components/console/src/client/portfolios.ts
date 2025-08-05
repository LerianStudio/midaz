import { PaginationDto } from '@/core/application/dto/pagination-dto'
import {
  PortfolioDto,
  PortfolioSearchParamDto
} from '@/core/application/dto/portfolio-dto'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import {
  getFetcher,
  postFetcher,
  patchFetcher,
  deleteFetcher,
  getPaginatedFetcher
} from '@/lib/fetcher'
import {
  useMutation,
  UseMutationOptions,
  useQuery
} from '@tanstack/react-query'

type UseCreatePortfolioProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

type UsePortfoliosWithAccountsProps = {
  organizationId: string
  ledgerId: string
  query: PortfolioSearchParamDto
}

export const usePortfoliosWithAccounts = ({
  organizationId,
  ledgerId,
  query,
  ...options
}: UsePortfoliosWithAccountsProps) => {
  return useQuery<PaginationDto<PortfolioDto>>({
    queryKey: [
      organizationId,
      ledgerId,
      'portfolios-with-accounts',
      Object.values(query)
    ],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/portfolios-accounts`,
      query
    ),
    enabled: !!organizationId && !!ledgerId,
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
    enabled: !!organizationId && !!ledgerId,
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
  return useQuery<PortfolioDto>({
    queryKey: [organizationId, ledgerId, 'portfolio', portfolioId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`
    ),
    ...options
  })
}
