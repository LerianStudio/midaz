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
import { isNil } from 'lodash'
import { EntityDataTable } from '@/components/entity-data-table'
import { Pagination, PaginationProps } from '@/components/pagination'
import {
  CursorPaginationDto,
  PaginationDto
} from '@/core/application/dto/pagination-dto'
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
import { CursorPagination } from '@/components/cursor-pagination'

type CursorPaginationControls = {
  hasNext: boolean
  hasPrev: boolean
  nextPage: () => void
  previousPage: () => void
  goToFirstPage: () => void
}
type AccountTypesDataTableProps = {
  accountTypes:
    | PaginationDto<AccountTypesDto>
    | CursorPaginationDto<AccountTypesDto>
    | undefined
  total?: number
  pagination?: PaginationProps
  handleCreate: () => void
  handleEdit: (accountType: AccountTypesDto) => void
  isLoading: boolean
  table: {
    getRowModel: () => {
      rows: { id: string; original: AccountTypesDto }[]
    }
  }
  onDelete: (id: string, accountType: AccountTypesDto) => void
  useCursorPagination?: boolean
  cursorPaginationControls?: CursorPaginationControls
}

type AccountTypesRowProps = {
  accountType: Row<AccountTypesDto>
  handleEdit: (accountType: AccountTypesDto) => void
  onDelete: (id: string, accountType: AccountTypesDto) => void
}

const AccountTypeRow: React.FC<AccountTypesRowProps> = ({
  accountType,
  handleEdit
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
        <TableCell>
          <div className="flex flex-col gap-1">
            <span className="font-medium">
              {accountType.original.description || '-'}
            </span>
          </div>
        </TableCell>
        <TableCell>{accountType.original.keyValue}</TableCell>
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
  handleEdit,
  useCursorPagination = false,
  cursorPaginationControls
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
              id: 'accountTypes.emptyResource',
              defaultMessage: "You haven't created any Account Types yet."
            })}
          >
            <Button onClick={handleCreate}>
              {intl.formatMessage({
                id: 'accountTypes.sheet.create.title',
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
                        id: 'accountTypes.field.name',
                        defaultMessage: 'Account Type Name'
                      })}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="text-muted-foreground h-4 w-4" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'accountTypes.field.name.tooltip',
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
                      id: 'accountTypes.field.description',
                      defaultMessage: 'Description'
                    })}
                  </TableHead>
                  <TableHead>
                    <div className="flex items-center gap-2">
                      {intl.formatMessage({
                        id: 'accountTypes.field.keyValue',
                        defaultMessage: 'Key Value'
                      })}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="text-muted-foreground h-4 w-4" />
                          </TooltipTrigger>
                          <TooltipContent>
                            {intl.formatMessage({
                              id: 'accountTypes.field.keyValue.tooltip',
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
          <EntityDataTable.FooterText>
            {intl.formatMessage(
              {
                id: 'ledgers.accounts.showing',
                defaultMessage:
                  '{number, plural, =0 {No accounts found} one {Showing {count} account} other {Showing {count} accounts}}.'
              },
              {
                number: accountTypes?.items.length,
                count: (
                  <span className="font-bold">
                    {accountTypes?.items.length}
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
            onFirst={cursorPaginationControls.goToFirstPage}
          />
        ) : (
          pagination &&
          total !== undefined && (
            <Pagination
              total={total}
              hasNextPage={
                accountTypes && accountTypes?.items.length < pagination.limit
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
