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
import {
  PaginationDto,
  CursorPaginationDto
} from '@/core/application/dto/pagination-dto'
import { CursorPagination } from '@/components/cursor-pagination'
import { MetadataTableCell } from '@/components/table/metadata-table-cell'
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
import { TransactionRoutesDto } from '@/core/application/dto/transaction-routes-dto'

type CursorPaginationControls = {
  hasNext: boolean
  hasPrev: boolean
  nextPage: () => void
  previousPage: () => void
  goToFirstPage: () => void
}

type TransactionRoutesDataTableProps = {
  transactionRoutes:
    | PaginationDto<TransactionRoutesDto>
    | CursorPaginationDto<TransactionRoutesDto>
    | undefined
  total?: number
  pagination?: PaginationProps
  handleCreate: () => void
  handleEdit: (transactionRoute: TransactionRoutesDto) => void
  isLoading: boolean
  table: {
    getRowModel: () => {
      rows: { id: string; original: TransactionRoutesDto }[]
    }
  }
  onDelete: (id: string, transactionRoute: TransactionRoutesDto) => void
  useCursorPagination?: boolean
  cursorPaginationControls?: CursorPaginationControls
}

type TransactionRoutesRowProps = {
  transactionRoute: Row<TransactionRoutesDto>
  handleEdit: (transactionRoute: TransactionRoutesDto) => void
  onDelete: (id: string, transactionRoute: TransactionRoutesDto) => void
}
const TransactionRoutesRow: React.FC<TransactionRoutesRowProps> = ({
  transactionRoute,
  handleEdit,
  onDelete
}) => {
  const intl = useIntl()
  return (
    <React.Fragment>
      <TableRow key={transactionRoute.id}>
        <TableCell>
          <div className="flex flex-col gap-1">
            <span className="font-medium">
              {transactionRoute.original.title}
            </span>
          </div>
        </TableCell>
        <TableCell>
          <div className="flex flex-col gap-1">
            <span className="font-medium">
              {truncate(transactionRoute.original.description, {
                length: 30
              }) || '-'}
            </span>
          </div>
        </TableCell>
        <TableCell>
          <TooltipProvider>
            <Tooltip delayDuration={300}>
              <TooltipTrigger>
                {intl.formatMessage(
                  {
                    id: 'transactionRoutes.field.operationRoutes.count',
                    defaultMessage: '{value} operation routes'
                  },
                  {
                    value:
                      transactionRoute.original.operationRoutes?.length || 0
                  }
                )}
              </TooltipTrigger>
              <TooltipContent>
                {transactionRoute.original.operationRoutes?.length > 0
                  ? transactionRoute.original.operationRoutes
                      .map((op) => op.title)
                      .join(', ')
                  : intl.formatMessage({
                      id: 'common.none',
                      defaultMessage: 'None'
                    })}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </TableCell>
        <MetadataTableCell metadata={transactionRoute.original.metadata!} />
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
                    ...transactionRoute.original,
                    entityId: transactionRoute.original.id
                  } as TransactionRoutesDto)
                }
              >
                {intl.formatMessage({
                  id: 'common.details',
                  defaultMessage: 'Details'
                })}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() =>
                  onDelete(
                    transactionRoute.original.id,
                    transactionRoute.original
                  )
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

export const TransactionRoutesDataTable: React.FC<
  TransactionRoutesDataTableProps
> = ({
  transactionRoutes,
  total,
  pagination,
  onDelete,
  handleCreate,
  handleEdit,
  useCursorPagination = false,
  cursorPaginationControls
}) => {
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = React.useState<any>([])

  const table = useReactTable({
    data: transactionRoutes?.items || [],
    columns: [
      { accessorKey: 'title' },
      { accessorKey: 'description' },
      { accessorKey: 'operationRoutes' },
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
        {isNil(transactionRoutes?.items) ||
        transactionRoutes.items.length === 0 ? (
          <EmptyResource
            message={intl.formatMessage({
              id: 'transactionRoutes.emptyResource',
              defaultMessage: "You haven't created any Transaction Routes yet."
            })}
          >
            <Button onClick={handleCreate}>
              {intl.formatMessage({
                id: 'transactionRoutes.sheet.create.title',
                defaultMessage: 'New Transaction Route'
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
                        id: 'transactionRoutes.field.title',
                        defaultMessage: 'Transaction Route Title'
                      })}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="text-muted-foreground h-4 w-4" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'transactionRoutes.field.title.tooltip',
                              defaultMessage:
                                'Enter the title of the transaction route'
                            })}
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'transactionRoutes.field.description',
                      defaultMessage: 'Description'
                    })}
                  </TableHead>
                  <TableHead>
                    <div className="flex items-center gap-2">
                      {intl.formatMessage({
                        id: 'transactionRoutes.field.operationRoutes',
                        defaultMessage: 'Operation Routes'
                      })}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="text-muted-foreground h-4 w-4" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'transactionRoutes.field.operationRoutes.tooltip',
                              defaultMessage:
                                'The operation routes associated with this transaction route'
                            })}
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
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
                {table.getRowModel().rows.map((transactionRoute) => (
                  <TransactionRoutesRow
                    key={transactionRoute.id}
                    transactionRoute={transactionRoute}
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
                id: 'transactionRoutes.showing',
                defaultMessage:
                  '{number, plural, =0 {No transaction routes found} one {Showing {count} transaction route} other {Showing {count} transaction routes}}.'
              },
              {
                number: transactionRoutes?.items.length,
                count: (
                  <span className="font-bold">
                    {transactionRoutes?.items.length}
                  </span>
                )
              }
            )}
          </EntityDataTable.FooterText>

          {useCursorPagination && cursorPaginationControls ? (
            <CursorPagination
              hasNext={cursorPaginationControls.hasNext}
              hasPrev={cursorPaginationControls.hasPrev}
              onNext={cursorPaginationControls.nextPage}
              onPrevious={cursorPaginationControls.previousPage}
            />
          ) : (
            pagination &&
            total !== undefined && (
              <Pagination
                total={total}
                hasNextPage={
                  transactionRoutes &&
                  transactionRoutes?.items?.length < pagination.limit
                }
                {...pagination}
              />
            )
          )}
        </EntityDataTable.Footer>
      </EntityDataTable.Root>
    </>
  )
}
