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
import { AccountTypesDto } from '@/core/application/dto/account-types-dto'
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
import { IdTableCell } from '@/components/table/id-table-cell'

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
  handleEdit
}) => {
  const intl = useIntl()
  console.log('operationRoute', operationRoute.original)
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
              {truncate(operationRoute.original.description, { length: 15 })}
            </span>
          </div>
        </TableCell>
        <TableCell>
          <TooltipProvider>
          <Tooltip delayDuration={300}>
            <TooltipTrigger>{truncate(operationRoute.original.account?.ruleType, { length: 16 })}</TooltipTrigger>
            <TooltipContent>
              {operationRoute.original.account?.ruleType === 'account_type' ? (
                <span>{operationRoute.original.account?.validIf.map((validIf: any) => validIf).join(', ')}</span>
              ) : (
                <span>{operationRoute.original.account?.validIf}</span>
              )}
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
              {/* <DropdownMenuItem
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
              </DropdownMenuItem> */}
            </DropdownMenuContent>
          </DropdownMenu>
        </TableCell>
      </TableRow>
    </React.Fragment>
  )
}

export const OperationRoutesDataTable: React.FC<OperationRoutesDataTableProps> = ({
  operationRoutes,
  total,  
  pagination,
  onDelete,
  handleCreate,   
  handleEdit,
}) => {
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = React.useState<any>([])

  const table = useReactTable({
    data: operationRoutes?.items || [],
    columns: [
      { accessorKey: 'title' },
      { accessorKey: 'description' },
      { accessorKey: 'keyValue' },
      { accessorKey: 'createdAt' },
      { accessorKey: 'actions' }
    ],
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    state: { columnFilters }
  })

  console.log('table', table)

  return (
    <>
      <EntityDataTable.Root>
        {isNil(operationRoutes?.items) || operationRoutes.items.length === 0 ? (
          <EmptyResource
            message={intl.formatMessage({
              id: 'account-types.emptyResource',
              defaultMessage: "You haven't created any Account Types yet."
            })}
          >
            <Button onClick={handleCreate}>
              {intl.formatMessage({
                id: 'account-types.sheet.create.title',
                defaultMessage: 'New Account Type'
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
                        id: 'account-types.field.name',
                        defaultMessage: 'Account Type Name'
                      })}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="text-muted-foreground h-4 w-4" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'account-types.field.name.tooltip',
                              defaultMessage:
                                'Enter the name of the account type'
                            })}
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'account-types.field.description',
                      defaultMessage: 'Description'
                    })}
                  </TableHead>
                  <TableHead>
                    <div className="flex items-center gap-2">
                      {intl.formatMessage({
                        id: 'account-types.field.keyValue',
                        defaultMessage: 'Key Value'
                      })}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="text-muted-foreground h-4 w-4" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'account-types.field.keyValue.tooltip',
                              defaultMessage:
                                'A unique key value identifier for the account type. Use only letters, numbers, underscores and hyphens.'
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
                id: 'ledgers.accounts.showing',
                defaultMessage:
                  '{number, plural, =0 {No accounts found} one {Showing {count} account} other {Showing {count} accounts}}.'
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
          <Pagination total={total} {...pagination} />
        </EntityDataTable.Footer>
      </EntityDataTable.Root>
    </>
  )
}
