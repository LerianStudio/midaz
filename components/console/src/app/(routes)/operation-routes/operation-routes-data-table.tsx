import React from 'react'
import { useIntl } from 'react-intl'
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { EmptyResource } from '@/components/empty-resource'
import { Button } from '@/components/ui/button'
import { MoreVertical } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { isNil, truncate } from 'lodash'
import { EntityDataTable } from '@/components/entity-data-table'
import { Pagination, PaginationProps } from '@/components/pagination'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { MetadataTableCell } from '@/components/table/metadata-table-cell'

const formatValidIf = (
  validIf: string | string[] | null | undefined
): string | undefined => {
  if (!validIf) return undefined

  if (typeof validIf === 'string') {
    return validIf.trim() || undefined
  }

  if (Array.isArray(validIf) && validIf.length > 0) {
    const cleanedItems = validIf
      .filter((item) => item?.trim())
      .map((item) => item.trim())

    if (cleanedItems.length === 0) return undefined
    if (cleanedItems.length === 1) return cleanedItems[0]

    return cleanedItems.join(', ')
  }

  return undefined
}

import {
  getCoreRowModel,
  getFilteredRowModel,
  Row,
  useReactTable
} from '@tanstack/react-table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { HelpCircle } from 'lucide-react'
import { OperationRoutesDto } from '@/core/application/dto/operation-routes-dto'

type OperationRoutesDataTableProps = {
  operationRoutes: PaginationDto<OperationRoutesDto> | undefined
  total: number
  pagination: PaginationProps
  handleCreate: () => void
  handleEdit: (operationRoute: OperationRoutesDto) => void
  isLoading: boolean
  table: {
    getRowModel: () => {
      rows: { id: string; original: OperationRoutesDto }[]
    }
  }
  onDelete: (id: string, operationRoute: OperationRoutesDto) => void
}

type OperationRoutesRowProps = {
  operationRoute: Row<OperationRoutesDto>
  handleEdit: (operationRoute: OperationRoutesDto) => void
  onDelete: (id: string, operationRoute: OperationRoutesDto) => void
}

const OperationRoutesRow: React.FC<OperationRoutesRowProps> = ({
  operationRoute,
  handleEdit,
  onDelete
}) => {
  const intl = useIntl()
  return (
    <React.Fragment>
      <TableRow key={operationRoute.id}>
        <TableCell>
          <div className="flex flex-col gap-1">
            <span className="font-medium">{operationRoute.original.title}</span>
          </div>
        </TableCell>
        <TableCell>
          <div className="flex flex-col gap-1">
            <span className="font-medium">
              {truncate(operationRoute.original.description, { length: 30 }) ||
                '-'}
            </span>
          </div>
        </TableCell>
        <TableCell>
          <TooltipProvider>
            <Tooltip delayDuration={300}>
              <TooltipTrigger>
                {truncate(operationRoute.original.operationType, {
                  length: 30
                })}
              </TooltipTrigger>
              <TooltipContent>
                {operationRoute.original.operationType}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </TableCell>
        <TableCell>
          <TooltipProvider>
            <Tooltip delayDuration={300}>
              <TooltipTrigger>
                {operationRoute?.original?.account?.ruleType}
              </TooltipTrigger>
              <TooltipContent>
                {formatValidIf(operationRoute?.original?.account?.validIf) ??
                  intl.formatMessage({
                    id: 'common.notApplicable',
                    defaultMessage: 'Not applicable'
                  })}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </TableCell>
        <MetadataTableCell metadata={operationRoute.original.metadata!} />
        <TableCell className="w-0">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="secondary" className="h-auto w-max p-2">
                <MoreVertical size={16} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onClick={() =>
                  handleEdit({
                    ...operationRoute.original,
                    entityId: operationRoute.original.id
                  } as OperationRoutesDto)
                }
              >
                {intl.formatMessage({
                  id: 'common.details',
                  defaultMessage: 'Details'
                })}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() =>
                  onDelete(operationRoute.original.id, operationRoute.original)
                }
              >
                {intl.formatMessage({
                  id: 'common.delete',
                  defaultMessage: 'Delete'
                })}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </TableCell>
      </TableRow>
    </React.Fragment>
  )
}

export const OperationRoutesDataTable: React.FC<
  OperationRoutesDataTableProps
> = ({
  operationRoutes,
  total,
  pagination,
  onDelete,
  handleCreate,
  handleEdit
}) => {
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = React.useState<any>([])

  const table = useReactTable({
    data: operationRoutes?.items || [],
    columns: [
      { accessorKey: 'title' },
      { accessorKey: 'description' },
      { accessorKey: 'operationType' },
      { accessorKey: 'ruleType' },
      { accessorKey: 'metadata' },
      { accessorKey: 'actions' }
    ],
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    state: { columnFilters }
  })

  return (
    <>
      <EntityDataTable.Root>
        {isNil(operationRoutes?.items) || operationRoutes.items.length === 0 ? (
          <EmptyResource
            message={intl.formatMessage({
              id: 'operationRoutes.emptyResource',
              defaultMessage: "You haven't created any Operation Routes yet."
            })}
          >
            <Button onClick={handleCreate}>
              {intl.formatMessage({
                id: 'operationRoutes.sheet.create.title',
                defaultMessage: 'New Operation Route'
              })}
            </Button>
          </EmptyResource>
        ) : (
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    <div className="flex items-center gap-2">
                      {intl.formatMessage({
                        id: 'operationRoutes.field.title',
                        defaultMessage: 'Title'
                      })}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="text-muted-foreground h-4 w-4" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'operationRoutes.field.title.tooltip',
                              defaultMessage: 'The title of the operation route'
                            })}
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'operationRoutes.field.description',
                      defaultMessage: 'Description'
                    })}
                  </TableHead>
                  <TableHead>
                    <div className="flex items-center gap-2">
                      {intl.formatMessage({
                        id: 'operationRoutes.field.operationType',
                        defaultMessage: 'Operation Type'
                      })}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="text-muted-foreground h-4 w-4" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'operationRoutes.field.operationType.tooltip',
                              defaultMessage:
                                'The type of operation (source or destination)'
                            })}
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'operationRoutes.field.ruleType',
                      defaultMessage: 'Rule Type'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.metadata',
                      defaultMessage: 'Metadata'
                    })}
                  </TableHead>
                  <TableHead className="w-0">
                    {intl.formatMessage({
                      id: 'common.actions',
                      defaultMessage: 'Actions'
                    })}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows.map((operationRoute) => (
                  <OperationRoutesRow
                    key={operationRoute.id}
                    operationRoute={operationRoute}
                    handleEdit={handleEdit}
                    onDelete={onDelete}
                  />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        <EntityDataTable.Footer>
          <EntityDataTable.FooterText>
            {intl.formatMessage(
              {
                id: 'operationRoutes.showing',
                defaultMessage:
                  '{number, plural, =0 {No operation routes found} one {Showing {count} operation route} other {Showing {count} operation routes}}.'
              },
              {
                number: operationRoutes?.items.length,
                count: (
                  <span className="font-bold">
                    {operationRoutes?.items.length}
                  </span>
                )
              }
            )}
          </EntityDataTable.FooterText>
          <Pagination
            total={total}
            hasNextPage={
              operationRoutes &&
              operationRoutes?.items?.length < pagination.limit
            }
            {...pagination}
          />
        </EntityDataTable.Footer>
      </EntityDataTable.Root>
    </>
  )
}
