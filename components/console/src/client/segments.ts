import { PaginationDto } from '@/core/application/dto/pagination-dto'
import {
  deleteFetcher,
  getPaginatedFetcher,
  patchFetcher,
  postFetcher
} from '@/lib/fetcher'
import { PaginationRequest } from '@/types/pagination-request-type'
import { SegmentType } from '@/types/segment-type'
import {
  keepPreviousData,
  useMutation,
  UseMutationOptions,
  useQuery
} from '@tanstack/react-query'

/**
 * TODO: Find a way to avoid the <any, any, any>
 */

type UseCreateSegmentProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

export const useCreateSegment = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreateSegmentProps) => {
  return useMutation<any, any, any>({
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/segments`
    ),
    ...options
  })
}

type UseListSegmentsProps = UseCreateSegmentProps & PaginationRequest

export const useListSegments = ({
  organizationId,
  ledgerId,
  limit,
  page,
  ...options
}: UseListSegmentsProps) => {
  return useQuery<PaginationDto<SegmentType>>({
    queryKey: [organizationId, ledgerId, limit, page],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/segments`,
      { limit, page }
    ),
    enabled: !!organizationId && !!ledgerId,
    placeholderData: keepPreviousData,
    ...options
  })
}

type UseUpdateSegmentProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
  segmentId: string
}

export const useUpdateSegment = ({
  organizationId,
  ledgerId,
  segmentId,
  ...options
}: UseUpdateSegmentProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId, segmentId],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`
    ),
    ...options
  })
}

type UseDeleteSegmentProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

export const useDeleteSegment = ({
  organizationId,
  ledgerId,
  ...options
}: UseDeleteSegmentProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId],
    mutationFn: deleteFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/segments`
    ),
    ...options
  })
}
