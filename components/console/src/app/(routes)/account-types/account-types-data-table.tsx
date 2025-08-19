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
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { isNil } from 'lodash'
import { EntityDataTable } from '@/components/entity-data-table'
import { Pagination, PaginationProps } from '@/components/pagination'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { IdTableCell } from '@/components/table/id-table-cell'
import { MetadataTableCell } from '@/components/table/metadata-table-cell'
import { AccountTypesDto } from '@/core/application/dto/account-types-dto'
import {
  getCoreRowModel,
  getFilteredRowModel,
  Row,
  useReactTable
} from '@tanstack/react-table'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { HelpCircle } from 'lucide-react'

type AccountTypesDataTableProps = {
  accountTypes: PaginationDto<AccountTypesDto> | undefined
  total: number
  pagination: PaginationProps
  handleCreate: () => void
  handleEdit: (accountType: AccountTypesDto) => void
  isLoading: boolean
  table: {
    getRowModel: () => {
      rows: { id: string; original: AccountTypesDto }[]
    }
  }
  onDelete: (id: string, accountType: AccountTypesDto) => void
}

type AccountTypesRowProps = {
  accountType: Row<AccountTypesDto>
  handleEdit: (accountType: AccountTypesDto) => void
  onDelete: (id: string, accountType: AccountTypesDto) => void
}

const AccountTypeRow: React.FC<AccountTypesRowProps> = ({
  accountType,
  handleEdit,
  onDelete
}) => {
  const intl = useIntl()

  return (
    <React.Fragment>
      <TableRow key={accountType.id}>
        <TableCell>
          <div className="flex flex-col gap-1">
            <span className="font-medium">{accountType.original.name}</span>
          </div>
        </TableCell>
        <IdTableCell id={accountType.original.description} />
        <TableCell>
            {accountType.original.keyValue}
        </TableCell>
        <MetadataTableCell metadata={accountType.original.metadata!} />
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
                    ...accountType.original,
                    entityId: accountType.original.id
                  } as AccountTypesDto)
                }
              >
                {intl.formatMessage({
                  id: 'common.details',
                  defaultMessage: 'Details'
                })}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={() => {
                  onDelete(accountType?.original?.id!, accountType?.original)
                }}
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

export const AccountTypesDataTable: React.FC<AccountTypesDataTableProps> = ({
  accountTypes,
  total,
  pagination,
  onDelete,
  handleCreate,
  handleEdit
}) => {
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = React.useState<any>([])

  const table = useReactTable({
    data: accountTypes?.items || [],
    columns: [
      { accessorKey: 'name' },
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

  return (
    <>
      <EntityDataTable.Root>
        {isNil(accountTypes?.items) || accountTypes.items.length === 0 ? (
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
                            <HelpCircle className="h-4 w-4 text-muted-foreground" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'account-types.field.name.tooltip',
                              defaultMessage: 'Enter the name of the account type'
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
                            <HelpCircle className="h-4 w-4 text-muted-foreground" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'account-types.field.keyValue.tooltip',
                              defaultMessage: 'A unique key value identifier for the account type. Use only letters, numbers, underscores and hyphens.'
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
                {table.getRowModel().rows.map((accountType) => (
                  <AccountTypeRow
                    key={accountType.id}
                    accountType={accountType}
                    handleEdit={handleEdit}
                    onDelete={onDelete}
                  />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        <EntityDataTable.Footer>
          <Pagination
            page={pagination.page}
            limit={pagination.limit}
            total={total}
            setPage={pagination.setPage}
            setLimit={pagination.setLimit}
            nextPage={pagination.nextPage}
            previousPage={pagination.previousPage}
          />
        </EntityDataTable.Footer>
      </EntityDataTable.Root>
    </>
  )
}
