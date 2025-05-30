import { AssetDto } from '@/core/application/dto/asset-dto'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import {
  deleteFetcher,
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

type UseCreateAssetProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

type UseListAssetsProps = UseCreateAssetProps & PaginationRequest

type UseUpdateAssetProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
  assetId: string
}

type UseDeleteAssetProps = UseCreateAssetProps

const useCreateAsset = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreateAssetProps) => {
  return useMutation<any, any, any>({
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/assets`
    ),
    ...options
  })
}

const useListAssets = ({
  organizationId,
  ledgerId,
  page,
  limit,
  ...options
}: UseListAssetsProps) => {
  return useQuery<PaginationDto<AssetDto>>({
    queryKey: ['assets', organizationId, ledgerId, page, limit],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/assets`,
      { page, limit }
    ),
    enabled: !!organizationId && !!ledgerId,
    ...options
  })
}

const useUpdateAsset = ({
  organizationId,
  ledgerId,
  assetId,
  ...options
}: UseUpdateAssetProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId, assetId],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`
    ),
    ...options
  })
}

const useDeleteAsset = ({
  organizationId,
  ledgerId,
  ...options
}: UseDeleteAssetProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId],
    mutationFn: deleteFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/assets`
    ),
    ...options
  })
}

export { useCreateAsset, useListAssets, useUpdateAsset, useDeleteAsset }
