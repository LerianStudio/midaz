import { EmptyResource } from '@/components/empty-resource'
import { EntityDataTable } from '@/components/entity-data-table'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import {
  getCoreRowModel,
  getFilteredRowModel,
  Row,
  useReactTable
} from '@tanstack/react-table'
import { isNil } from 'lodash'
import { MoreVertical } from 'lucide-react'
import React from 'react'
import { useIntl } from 'react-intl'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { Pagination, PaginationProps } from '@/components/pagination'
import { FormProvider, UseFormReturn } from 'react-hook-form'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { IdTableCell } from '@/components/table/id-table-cell'
import { PortfolioType } from '@/types/portfolio-type'
import { MetadataTableCell } from '@/components/table/metadata-table-cell'

type PortfoliosDataTableProps = {
  portfolios: PaginationDto<PortfolioType> | undefined
  form: UseFormReturn<any>
  total: number
  pagination: PaginationProps
  handleCreate: () => void
  handleDialogOpen: (id: string) => void
  handleEdit: (portfolio: PortfolioType) => void
}

type PortfoliosRowProps = {
  portfolio: Row<PortfolioType>
  handleDialogOpen: (id: string) => void
  handleEdit: (portfolio: PortfolioType) => void
}

const PortfolioRow: React.FC<PortfoliosRowProps> = ({
  portfolio,
  handleDialogOpen,
  handleEdit
}) => {
  const intl = useIntl()

  return (
    <React.Fragment>
      <TableRow key={portfolio.id}>
        <IdTableCell id={portfolio.original.id} />
        <TableCell>{portfolio.original.name}</TableCell>
        <TableCell>
          {intl.formatMessage(
            {
              id: 'common.table.accounts',
              defaultMessage:
                '{number, plural, =0 {No accounts} one {# account} other {# accounts}}'
            },
            {
              number: portfolio.original.accounts?.length || 0
            }
          )}
        </TableCell>

        <MetadataTableCell metadata={portfolio.original.metadata} />

        <TableCell className="w-0">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="secondary" className="h-auto w-max p-2">
                <MoreVertical size={16} onClick={() => {}} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onClick={() =>
                  handleEdit({
                    ...portfolio.original,
                    entityId: portfolio.original.id
                  } as PortfolioType)
                }
              >
                {intl.formatMessage({
                  id: `common.details`,
                  defaultMessage: 'Details'
                })}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={() => {
                  handleDialogOpen(portfolio?.original?.id!)
                }}
              >
                {intl.formatMessage({
                  id: `common.delete`,
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

export const PortfoliosDataTable: React.FC<PortfoliosDataTableProps> = (
  props
) => {
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = React.useState<any>([])

  const {
    portfolios,
    handleCreate,
    handleDialogOpen,
    handleEdit,
    form,
    pagination,
    total
  } = props

  const table = useReactTable({
    data: portfolios?.items!,
    columns: [
      {
        accessorKey: 'name'
      }
    ],
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    state: {
      columnFilters
    }
  })

  return (
    <FormProvider {...form}>
      <div className="mb-4 flex justify-end">
        <PaginationLimitField control={form.control} />
      </div>

      <EntityDataTable.Root>
        {isNil(portfolios?.items) || portfolios.items.length === 0 ? (
          <EmptyResource
            message={intl.formatMessage({
              id: 'ledgers.portfolios.emptyResource',
              defaultMessage: "You haven't created any Portfolios yet"
            })}
          >
            <Button onClick={handleCreate}>
              {intl.formatMessage({
                id: 'common.new.portfolio',
                defaultMessage: 'New Portfolio'
              })}
            </Button>
          </EmptyResource>
        ) : (
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.id',
                      defaultMessage: 'ID'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.name',
                      defaultMessage: 'Name'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.accounts',
                      defaultMessage: 'Accounts'
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
                {table.getRowModel().rows.map((portfolio) => (
                  <PortfolioRow
                    key={portfolio.id}
                    portfolio={portfolio}
                    handleDialogOpen={handleDialogOpen}
                    handleEdit={handleEdit}
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
                id: 'ledgers.portfolios.showing',
                defaultMessage:
                  '{number, plural, =0 {No portfolios found} one {Showing {count} portfolio} other {Showing {count} portfolios}}.'
              },
              {
                number: portfolios?.items?.length || 0,
                count: (
                  <span className="font-bold">{portfolios?.items?.length}</span>
                )
              }
            )}
          </EntityDataTable.FooterText>
          <Pagination total={total} {...pagination} />
        </EntityDataTable.Footer>
      </EntityDataTable.Root>
    </FormProvider>
  )
}
