import React from 'react'
import { defineMessages, useIntl } from 'react-intl'
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
import { capitalizeFirstLetter } from '@/helpers'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { isNil } from 'lodash'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { FormProvider, UseFormReturn } from 'react-hook-form'
import { EntityDataTable } from '@/components/entity-data-table'
import { Pagination, PaginationProps } from '@/components/pagination'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { AssetResponseDto } from '@/core/application/dto/asset-dto'

type AssetsTableProps = {
  assets: PaginationDto<AssetResponseDto> | undefined
  table: {
    getRowModel: () => {
      rows: { id: string; original: AssetResponseDto }[]
    }
  }
  handleDialogOpen: (id: string, name: string) => void
  handleCreate: () => void
  handleEdit: (asset: AssetResponseDto) => void
  form: UseFormReturn<any>
  total: number
  pagination: PaginationProps
}

type AssetRowProps = {
  asset: { id: string; original: AssetResponseDto }
  handleDialogOpen: (id: string, name: string) => void
  handleEdit: (asset: AssetResponseDto) => void
}

const AssetRow: React.FC<AssetRowProps> = ({
  asset,
  handleDialogOpen,
  handleEdit
}) => {
  const intl = useIntl()
  const metadataCount = Object.entries(asset.original.metadata || []).length

  const assetTypesMessages = defineMessages({
    crypto: {
      id: 'assets.sheet.select.crypto',
      defaultMessage: 'Crypto'
    },
    commodity: {
      id: 'assets.sheet.select.commodity',
      defaultMessage: 'Commodity'
    },
    currency: {
      id: 'assets.sheet.select.currency',
      defaultMessage: 'Currency'
    },
    others: {
      id: 'assets.sheet.select.others',
      defaultMessage: 'Others'
    }
  })

  return (
    <TableRow key={asset.id}>
      <TableCell>{asset.original.name}</TableCell>
      <TableCell>
        {capitalizeFirstLetter(
          intl.formatMessage(
            assetTypesMessages[
              asset.original.type as keyof typeof assetTypesMessages
            ]
          )
        )}
      </TableCell>
      <TableCell>{asset.original.code}</TableCell>
      <TableCell>
        {intl.formatMessage(
          {
            id: 'common.table.metadata',
            defaultMessage:
              '{number, plural, =0 {-} one {# record} other {# records}}'
          },
          {
            number: metadataCount
          }
        )}
      </TableCell>
      <TableCell align="center">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="secondary" className="h-auto w-max p-2">
              <MoreVertical size={16} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => handleEdit(asset.original)}>
              {intl.formatMessage({
                id: `common.details`,
                defaultMessage: 'Details'
              })}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() =>
                handleDialogOpen(
                  asset.original.id || '',
                  asset.original.name || ''
                )
              }
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
  )
}

export const AssetsDataTable: React.FC<AssetsTableProps> = (props) => {
  const intl = useIntl()

  const {
    assets,
    table,
    handleDialogOpen,
    handleCreate,
    handleEdit,
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
        {isNil(assets?.items) || assets.items.length === 0 ? (
          <EmptyResource
            message={intl.formatMessage({
              id: 'ledgers.assets.emptyResource',
              defaultMessage: 'You have not created any assets yet.'
            })}
          >
            <Button variant="default" onClick={handleCreate}>
              {intl.formatMessage({
                id: 'ledgers.assets.emptyResource.createButton',
                defaultMessage: 'New Asset'
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
                      id: 'common.name',
                      defaultMessage: 'Name'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.type',
                      defaultMessage: 'Type'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.code',
                      defaultMessage: 'Code'
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
                {table.getRowModel().rows.map((asset) => {
                  return (
                    <AssetRow
                      key={asset.id}
                      asset={asset}
                      handleDialogOpen={handleDialogOpen}
                      handleEdit={handleEdit}
                    />
                  )
                })}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        <EntityDataTable.Footer>
          <EntityDataTable.FooterText>
            {intl.formatMessage(
              {
                id: 'ledgers.assets.showing',
                defaultMessage:
                  '{number, plural, =0 {No asset found} one {Showing {count} asset} other {Showing {count} assets}}.'
              },
              {
                number: assets?.items?.length,
                count: (
                  <span className="font-bold">{assets?.items?.length}</span>
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
