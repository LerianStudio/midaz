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
import { MoreVertical, Minus, HelpCircle } from 'lucide-react'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { Arrow } from '@radix-ui/react-tooltip'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { isNil } from 'lodash'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { EntityDataTable } from '@/components/entity-data-table'
import { FormProvider, UseFormReturn } from 'react-hook-form'
import { Table as ReactTableType } from '@tanstack/react-table'
import { LedgerResponseDto } from '@/core/application/dto/ledger-dto'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { Pagination, PaginationProps } from '@/components/pagination'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { AssetsSheet } from '../assets/assets-sheet'
import { IdTableCell } from '@/components/table/id-table-cell'

type LedgersTableProps = {
  ledgers: PaginationDto<LedgerResponseDto> | undefined
  table: ReactTableType<LedgerResponseDto>
  handleDialogOpen: (id: string, name: string) => void
  handleCreate: () => void
  handleEdit: (ledger: LedgerResponseDto) => void
  refetch: () => void
  form: UseFormReturn<any>
  total: number
  pagination: PaginationProps
}

type LedgerRowProps = {
  ledger: { id: string; original: LedgerResponseDto }
  handleDialogOpen: (id: string, name: string) => void
  handleEdit: (ledger: LedgerResponseDto) => void
  refetch: () => void
}

const LedgerRow: React.FC<LedgerRowProps> = ({
  ledger,
  handleDialogOpen,
  handleEdit,
  refetch
}) => {
  const intl = useIntl()
  const metadataCount = Object.entries(ledger.original.metadata || []).length
  const assetsItems = ledger.original.assets || []
  const { handleCreate, sheetProps } = useCreateUpdateSheet<any>()

  const renderAssets = () => {
    if (assetsItems.length === 1) {
      return <p>{assetsItems[0].code}</p>
    }

    if (assetsItems.length > 1) {
      return (
        <div className="flex items-center gap-1">
          <p>
            {intl.formatMessage(
              {
                id: 'ledgers.assets.count',
                defaultMessage: '{count} assets'
              },
              { count: assetsItems.length }
            )}
          </p>
          <TooltipProvider>
            <Tooltip delayDuration={300}>
              <TooltipTrigger asChild className="flex self-end">
                <span className="cursor-pointer">
                  <HelpCircle size={16} />
                </span>
              </TooltipTrigger>
              <TooltipContent className="max-w-80">
                <p className="text-shadcn-400">
                  {assetsItems.map((asset) => asset.code).join(', ')}
                </p>
                <Arrow height={8} width={15} />
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      )
    }

    return (
      <Button
        variant="link"
        className="h-fit px-0 py-0"
        onClick={(e) => {
          e.stopPropagation()
          handleCreate()
        }}
      >
        <p className="text-shadcn-600 underline">
          {intl.formatMessage({
            id: 'common.add',
            defaultMessage: 'Add'
          })}
        </p>
      </Button>
    )
  }

  return (
    <React.Fragment>
      <TableRow key={ledger.id}>
        <IdTableCell id={ledger.original.id} />
        <TableCell>{ledger.original.name}</TableCell>
        <TableCell>{renderAssets()}</TableCell>
        <TableCell>
          {metadataCount === 0 ? (
            <Minus size={20} />
          ) : (
            intl.formatMessage(
              {
                id: 'common.table.metadata',
                defaultMessage:
                  '{number, plural, =0 {-} one {# record} other {# records}}'
              },
              {
                number: metadataCount
              }
            )
          )}
        </TableCell>
        <TableCell className="w-0">
          <div className="flex justify-end">
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
                <DropdownMenuItem onClick={() => handleEdit(ledger.original)}>
                  {intl.formatMessage({
                    id: `common.edit`,
                    defaultMessage: 'Edit'
                  })}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  data-testid="delete"
                  onClick={(e) => {
                    e.stopPropagation()
                    handleDialogOpen(
                      ledger.original.id || '',
                      ledger.original.name || ''
                    )
                  }}
                >
                  {intl.formatMessage({
                    id: `common.delete`,
                    defaultMessage: 'Delete'
                  })}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </TableCell>
      </TableRow>

      <AssetsSheet
        onSuccess={refetch}
        ledgerId={ledger.original.id!}
        {...sheetProps}
      />
    </React.Fragment>
  )
}

export const LedgersDataTable: React.FC<LedgersTableProps> = (props) => {
  const intl = useIntl()

  const {
    ledgers,
    table,
    handleDialogOpen,
    handleCreate,
    handleEdit,
    refetch,
    form,
    pagination,
    total
  } = props

  return (
    <FormProvider {...form}>
      <div className="mb-4 flex justify-end">
        <PaginationLimitField control={form.control} />
      </div>

      <EntityDataTable.Root>
        {isNil(ledgers?.items) || ledgers.items.length === 0 ? (
          <EmptyResource
            message={intl.formatMessage({
              id: 'ledgers.emptyResource',
              defaultMessage: "You haven't created any Ledger yet"
            })}
          >
            <Button variant="default" onClick={handleCreate}>
              {intl.formatMessage({
                id: 'ledgers.emptyResource.createButton',
                defaultMessage: 'New Ledger'
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
                      id: 'entity.ledger.name',
                      defaultMessage: 'Ledger Name'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.assets',
                      defaultMessage: 'Assets'
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
                {table.getRowModel().rows.map((ledger) => (
                  <LedgerRow
                    key={ledger.id}
                    ledger={ledger}
                    handleDialogOpen={handleDialogOpen}
                    handleEdit={handleEdit}
                    refetch={refetch}
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
                id: 'ledgers.showing',
                defaultMessage:
                  'Showing {count} {number, plural, =0 {ledgers} one {ledger} other {ledgers}}.'
              },
              {
                number: ledgers?.items?.length,
                count: (
                  <span className="font-bold">{ledgers?.items?.length}</span>
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
