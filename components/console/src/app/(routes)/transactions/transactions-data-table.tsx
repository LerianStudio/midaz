import { EmptyResource } from '@/components/empty-resource'
import { EntityDataTable } from '@/components/entity-data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
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
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { capitalizeFirstLetter } from '@/helpers'
import {
  getCoreRowModel,
  getFilteredRowModel,
  Row,
  useReactTable
} from '@tanstack/react-table'
import { isNil } from 'lodash'
import { HelpCircle, MoreVertical } from 'lucide-react'
import Link from 'next/link'
import React from 'react'
import { defineMessages, useIntl } from 'react-intl'
import dayjs from 'dayjs'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { Pagination, PaginationProps } from '@/components/pagination'
import { FormProvider, UseFormReturn } from 'react-hook-form'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { IdTableCell } from '@/components/table/id-table-cell'
import {
  TransactionOperationDto,
  TransactionDto
} from '@/core/application/dto/transaction-dto'

type TransactionsDataTableProps = {
  transactions: PaginationDto<TransactionDto> | undefined
  form: UseFormReturn<any>
  total: number
  pagination: PaginationProps
  onCreateTransaction: () => void
}

type TransactionsRowProps = {
  transaction: Row<TransactionDto>
}

const multipleItemsMessages = defineMessages({
  source: {
    id: 'transactions.multiple.source',
    defaultMessage: '{count} sources'
  },
  destination: {
    id: 'transactions.multiple.destination',
    defaultMessage: '{count} destinations'
  }
})

const getBadgeVariant = (status: string) =>
  status === Status.APPROVED ? 'active' : 'inactive'

enum Status {
  APPROVED = 'APPROVED',
  CANCELED = 'CANCELED'
}

const statusMessages = defineMessages({
  [Status.APPROVED]: {
    id: 'status.approved',
    defaultMessage: 'Approved'
  },
  [Status.CANCELED]: {
    id: 'status.canceled',
    defaultMessage: 'Canceled'
  }
})

const TransactionRow: React.FC<TransactionsRowProps> = ({ transaction }) => {
  const intl = useIntl()
  const {
    status: { code },
    createdAt,
    asset,
    source = [],
    destination = []
  } = transaction.original

  const badgeVariant = getBadgeVariant(code)

  const renderItemsList = (
    items: TransactionOperationDto[],
    type: 'source' | 'destination'
  ) => {
    if (items.length === 1) {
      return <p>{String(items[0].accountAlias)}</p>
    }
    if (items.length === 0) {
      return null
    }
    const messageDescriptor = multipleItemsMessages[type]
    const labelWithCount = intl.formatMessage(messageDescriptor, {
      count: items.length
    })
    return (
      <div className="flex items-center gap-1">
        <p>{labelWithCount}</p>
        <TooltipProvider>
          <Tooltip delayDuration={300}>
            <TooltipTrigger asChild className="flex self-end">
              <HelpCircle size={16} className="cursor-pointer" />
            </TooltipTrigger>
            <TooltipContent className="max-w-80">
              <p className="text-shadcn-400">
                {items.map((item) => item.accountAlias).join(', ')}
              </p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>
    )
  }

  const renderSource = renderItemsList(source, 'source')
  const renderDestination = renderItemsList(destination, 'destination')

  return (
    <React.Fragment>
      <TableRow key={transaction.id}>
        <TableCell>{dayjs(createdAt).format('L HH:mm')}</TableCell>
        <IdTableCell id={transaction.original.id} />
        <TableCell>{renderSource}</TableCell>
        <TableCell>{renderDestination}</TableCell>
        <TableCell align="center">
          <Badge variant={badgeVariant}>
            {capitalizeFirstLetter(
              intl.formatMessage(
                statusMessages[code as keyof typeof statusMessages]
              )
            )}
          </Badge>
        </TableCell>
        <TableCell className="text-base font-medium text-zinc-600">
          <span className="mr-2 text-xs font-normal">{asset}</span>
          {capitalizeFirstLetter(intl.formatNumber(transaction.original.value))}
        </TableCell>
        <TableCell align="center">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="secondary"
                className="h-auto w-max p-2"
                data-testid="actions"
              >
                <MoreVertical size={16} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <Link href={`/transactions/${transaction.original.id}`}>
                <DropdownMenuItem>
                  {intl.formatMessage({
                    id: 'common.seeDetails',
                    defaultMessage: 'See details'
                  })}
                </DropdownMenuItem>
              </Link>
            </DropdownMenuContent>
          </DropdownMenu>
        </TableCell>
      </TableRow>
    </React.Fragment>
  )
}

export const TransactionsDataTable = ({
  transactions,
  form,
  total,
  pagination,
  onCreateTransaction
}: TransactionsDataTableProps) => {
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = React.useState<any>([])

  const table = useReactTable({
    data: transactions?.items || [],
    columns: [
      { accessorKey: 'data' },
      { accessorKey: 'id' },
      { accessorKey: 'source' },
      { accessorKey: 'destination' },
      { accessorKey: 'status' },
      { accessorKey: 'value' },
      { accessorKey: 'actions' }
    ],
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    state: { columnFilters }
  })

  return (
    <FormProvider {...form}>
      <div className="mb-4 flex justify-end">
        <PaginationLimitField control={form.control} />
      </div>

      <EntityDataTable.Root>
        {isNil(transactions?.items) || transactions.items.length === 0 ? (
          <EmptyResource
            message={intl.formatMessage({
              id: 'transactions.emptyResource',
              defaultMessage: "You haven't created any transactions yet."
            })}
          >
            <Button variant="default" onClick={onCreateTransaction}>
              {intl.formatMessage({
                id: 'transactions.create.title',
                defaultMessage: 'New Transaction'
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
                      id: 'entity.transactions.data',
                      defaultMessage: 'Data'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.id',
                      defaultMessage: 'ID'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'entity.transactions.source',
                      defaultMessage: 'Source'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'entity.transactions.destination',
                      defaultMessage: 'Destination'
                    })}
                  </TableHead>
                  <TableHead className="text-center">
                    {intl.formatMessage({
                      id: 'common.status',
                      defaultMessage: 'Status'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'entity.transactions.value',
                      defaultMessage: 'Value'
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
                {table.getRowModel().rows.map((transaction) => (
                  <TransactionRow
                    key={transaction.id}
                    transaction={transaction}
                  />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        <EntityDataTable.Footer className="flex items-center justify-between py-4">
          <EntityDataTable.FooterText>
            {intl.formatMessage(
              {
                id: 'transactions.showing',
                defaultMessage:
                  '{number, plural, =0 {No transaction found} one {Showing {count} transaction} other {Showing {count} transactions}}.'
              },
              {
                number: transactions?.items?.length,
                count: (
                  <span className="font-bold">
                    {transactions?.items?.length}
                  </span>
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
