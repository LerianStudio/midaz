import {
  OperationRoutesDto,
  OperationRoutesSearchParamDto,
  CreateOperationRoutesDto,
  UpdateOperationRoutesDto
} from '@/core/application/dto/operation-routes-dto'
import {
  PaginationDto,
  CursorPaginationDto
} from '@/core/application/dto/pagination-dto'
import {
  deleteFetcher,
  getFetcher,
  getPaginatedFetcher,
  getCursorPaginatedFetcher,
  patchFetcher,
  postFetcher
} from '@/lib/fetcher'
import {
  useMutation,
  UseMutationOptions,
  useQuery,
  UseQueryOptions
} from '@tanstack/react-query'

type UseListOperationRoutesProps = {
  organizationId: string
  ledgerId: string
  query?: OperationRoutesSearchParamDto
  enabled?: boolean
} & Omit<
  UseQueryOptions<PaginationDto<OperationRoutesDto>>,
  'queryKey' | 'queryFn'
>

export const useListOperationRoutes = ({
  organizationId,
  ledgerId,
  query,
  enabled = true,
  ...options
}: UseListOperationRoutesProps) => {
  return useQuery<PaginationDto<OperationRoutesDto>>({
    queryKey: [
      organizationId,
      ledgerId,
      'operation-routes',
      ...Object.values(query ?? {})
    ],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes`,
      query
    ),
    enabled: !!organizationId && !!ledgerId && enabled,
    ...options
  })
}

type UseOperationRouteProps = {
  organizationId: string
  ledgerId: string
  operationRouteId: string
  enabled?: boolean
} & Omit<UseQueryOptions<OperationRoutesDto>, 'queryKey' | 'queryFn'>

export const useGetOperationRoute = ({
  organizationId,
  ledgerId,
  operationRouteId,
  ...options
}: UseOperationRouteProps) => {
  return useQuery<OperationRoutesDto>({
    queryKey: [organizationId, ledgerId, 'operation-routes', operationRouteId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes/${operationRouteId}`
    ),
    ...options
  })
}

type UseCreateOperationRouteProps = {
  organizationId: string
  ledgerId: string
} & UseMutationOptions<OperationRoutesDto, any, CreateOperationRoutesDto>

export const useCreateOperationRoute = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreateOperationRouteProps) => {
  return useMutation<OperationRoutesDto, any, CreateOperationRoutesDto>({
    mutationKey: ['create-operation-route', organizationId, ledgerId],
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes`
    ),
    ...options
  })
}

type UseUpdateOperationRouteProps = {
  organizationId: string
  ledgerId: string
  operationRouteId: string
} & UseMutationOptions<OperationRoutesDto, any, UpdateOperationRoutesDto>

export const useUpdateOperationRoute = ({
  organizationId,
  ledgerId,
  operationRouteId,
  ...options
}: UseUpdateOperationRouteProps) => {
  return useMutation<OperationRoutesDto, any, UpdateOperationRoutesDto>({
    mutationKey: [
      'update-operation-route',
      organizationId,
      ledgerId,
      operationRouteId
    ],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes/${operationRouteId}`
    ),
    ...options
  })
}

type UseDeleteOperationRouteProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
  operationRouteId: string
}

export const useDeleteOperationRoute = ({
  organizationId,
  ledgerId,
  operationRouteId,
  ...options
}: UseDeleteOperationRouteProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId, operationRouteId],
    mutationFn: deleteFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes`
    ),
    ...options
  })
}

type UseListOperationRoutesCursorProps = {
  organizationId: string
  ledgerId: string
  cursor?: string
  limit?: number
  sortOrder?: 'asc' | 'desc'
  sortBy?: 'id' | 'title' | 'createdAt' | 'updatedAt'
  id?: string
  title?: string
  enabled?: boolean
}

export const useListOperationRoutesCursor = ({
  organizationId,
  ledgerId,
  cursor,
  limit = 10,
  sortOrder = 'desc',
  sortBy = 'createdAt',
  id,
  title,
  enabled = true,
  ...options
}: UseListOperationRoutesCursorProps) => {
  const params: OperationRoutesSearchParamDto = {
    cursor,
    limit,
    sortOrder,
    sortBy,
    id,
    title
  }

  // Filter out undefined values
  const cleanParams = Object.fromEntries(
    Object.entries(params).filter(([_, value]) => value !== undefined)
  )

  return useQuery<CursorPaginationDto<OperationRoutesDto>>({
    queryKey: [
      organizationId,
      ledgerId,
      'operation-routes-cursor',
      cleanParams
    ],
    queryFn: getCursorPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes`,
      cleanParams
    ),
    enabled: !!organizationId && !!ledgerId && enabled,
    ...options
  })
}
