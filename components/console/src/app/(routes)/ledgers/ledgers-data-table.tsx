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
import { MoreVertical, RefreshCw, Check } from 'lucide-react'
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
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable
} from '@tanstack/react-table'
import { LedgerDto } from '@/core/application/dto/ledger-dto'
import { Pagination, PaginationProps } from '@/components/pagination'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { AssetsSheet } from '../assets/assets-sheet'
import { IdTableCell } from '@/components/table/id-table-cell'
import { useOrganization } from '@lerianstudio/console-layout'
import { cn } from '@/lib/utils'
import { AssetTableCell } from './asset-table-cell'
import { NameTableCell } from '@/components/table/name-table-cell'
import {
  TooltipProvider,
  Tooltip,
  TooltipTrigger,
  TooltipContent
} from '@/components/ui/tooltip'
import { useRouter } from 'next/navigation'

type LedgerRowProps = {
  ledger: { id: string; original: LedgerDto }
  active?: boolean
  handleDialogOpen: (id: string, name: string) => void
  handleEdit: (ledger: LedgerDto) => void
  refetch: () => void
}

const LedgerRow: React.FC<LedgerRowProps> = ({
  ledger,
  active,
  handleDialogOpen,
  handleEdit,
  refetch
}) => {
  const intl = useIntl()
  const { setLedger } = useOrganization()
  const { handleCreate, sheetProps } = useCreateUpdateSheet<any>()
  const router = useRouter()

  const handleAssetsClick = () => {
    setLedger(ledger.original)
    router.push('/assets')
  }

  return (
    <React.Fragment>
      <TableRow key={ledger.id} active={active}>
        <TableCell>
          {active && (
            <div className="flex size-8 items-center justify-center">
              <Check className="text-shadcn-500 size-4" />
            </div>
          )}
          {!active && (
            <TooltipProvider>
              <Tooltip delayDuration={500}>
                <TooltipTrigger asChild>
                  <Button
                    className="size-8 p-0"
                    variant="outline"
                    onClick={() => setLedger(ledger.original)}
                  >
                    <RefreshCw className="size-3 shrink-0" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  {intl.formatMessage({
                    id: `ledgers.useLedger`,
                    defaultMessage: 'Switch to this ledger'
                  })}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
        </TableCell>
        <NameTableCell
          className={cn({ 'font-semibold text-neutral-700': active })}
          name={
            active
              ? intl.formatMessage(
                  {
                    id: 'ledgers.current.name',
                    defaultMessage: '{name} <b>(current)</b>'
                  },
                  {
                    name: ledger.original.name,
                    b: (chunks) => <i>{chunks}</i>
                  }
                )
              : ledger.original.name
          }
          onClick={() => handleEdit(ledger.original)}
        />
        <IdTableCell id={ledger.original.id} />
        <AssetTableCell
          assets={ledger.original.assets || []}
          onCreate={handleCreate}
          onClick={handleAssetsClick}
        />
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
                    id: `common.details`,
                    defaultMessage: 'Details'
                  })}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                {!active && (
                  <>
                    <DropdownMenuItem
                      onClick={() => setLedger(ledger.original)}
                    >
                      {intl.formatMessage({
                        id: `ledgers.useLedger`,
                        defaultMessage: 'Switch to this ledger'
                      })}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                  </>
                )}
                <DropdownMenuItem
                  data-testid="delete"
                  onClick={(e) => {
                    e.stopPropagation()
                    handleDialogOpen(ledger.original.id!, ledger.original.name!)
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
        _ledgerId={ledger.original.id!}
        {...sheetProps}
      />
    </React.Fragment>
  )
}

type LedgersTableProps = {
  ledgers: PaginationDto<LedgerDto> | undefined
  handleDialogOpen: (id: string, name: string) => void
  handleCreate: () => void
  handleEdit: (ledger: LedgerDto) => void
  refetch: () => void
  total: number
  pagination: PaginationProps
}

export const LedgersDataTable: React.FC<LedgersTableProps> = (props) => {
  const intl = useIntl()
  const { currentLedger } = useOrganization()

  const {
    ledgers,
    handleDialogOpen,
    handleCreate,
    handleEdit,
    refetch,
    pagination,
    total
  } = props

  const items = React.useMemo(
    () =>
      ledgers?.items?.filter((ledger) => ledger.id !== currentLedger.id) ?? [],
    [ledgers?.items, currentLedger.id]
  )

  const table = useReactTable({
    data: items,
    columns: [
      { accessorKey: 'name' },
      { accessorKey: 'id' },
      { accessorKey: 'assets' },
      { accessorKey: 'actions' }
    ],
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel()
  })

  return (
    <>
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
                id: 'ledgers.sheetCreate.title',
                defaultMessage: 'New Ledger'
              })}
            </Button>
          </EmptyResource>
        ) : (
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-0" />
                  <TableHead>
                    {intl.formatMessage({
                      id: 'entity.ledger.name',
                      defaultMessage: 'Ledger Name'
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
                      id: 'common.assets',
                      defaultMessage: 'Assets'
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
                {currentLedger && (
                  <LedgerRow
                    active
                    ledger={{ id: '1', original: currentLedger }}
                    handleDialogOpen={handleDialogOpen}
                    handleEdit={handleEdit}
                    refetch={refetch}
                  />
                )}
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
                number: items?.length,
                count: <span className="font-bold">{items?.length}</span>
              }
            )}
          </EntityDataTable.FooterText>
          <Pagination total={total} {...pagination} />
        </EntityDataTable.Footer>
      </EntityDataTable.Root>
    </>
  )
}
