import React from 'react'
import { useIntl } from 'react-intl'

import {
  Table,
  TableContainer,
  TableHead,
  TableRow,
  TableHeader,
  TableCell,
  TableBody
} from '@/components/ui/table'
import { MoreVertical } from 'lucide-react'
import { isNil } from 'lodash'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { AccountType } from '@/types/accounts-type'
import { MetadataTableCell } from '@/components/table/metadata-table-cell'
import { EntityDataTable } from '@/components/entity-data-table'
import { EmptyResource } from '@/components/empty-resource'
import { Pagination, PaginationProps } from '@/components/pagination'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { FormProvider, UseFormReturn } from 'react-hook-form'
import { IdTableCell } from '@/components/table/id-table-cell'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { AlertCircle } from 'lucide-react'
import { useRouter } from 'next/navigation'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'

type AccountsTableProps = {
  accounts: { items: AccountType[] }
  isLoading: boolean
  table: {
    getRowModel: () => {
      rows: { id: string; original: AccountType }[]
    }
  }
  onDelete: (id: string, account: AccountType) => void
  refetch: () => void
  handleCreate: () => void
  handleEdit: (account: AccountType) => void
  total: number
  pagination: PaginationProps
  form: UseFormReturn<any>
  hasAssets: boolean
}

type AccountRowProps = {
  account: { id: string; original: AccountType }
  handleEdit: (account: AccountType) => void
  onDelete: (id: string, account: AccountType) => void
}

const AccountRow: React.FC<AccountRowProps> = ({
  account,
  handleEdit,
  onDelete
}) => {
  const intl = useIntl()
  const isExternal = account.original.alias?.includes('@external/')

  return (
    <TableRow key={account.id}>
      <IdTableCell id={account.original.id} />
      <TableCell>{account.original.name}</TableCell>
      <TableCell align="center">{account.original.assetCode}</TableCell>
      <MetadataTableCell align="center" metadata={account.original.metadata} />
      <TableCell align="center">
        {isExternal && '-'}
        {!isExternal &&
          (account.original.portfolio?.name ?? (
            <Button variant="link" onClick={() => handleEdit(account.original)}>
              {intl.formatMessage({
                id: 'common.link',
                defaultMessage: 'Link'
              })}
            </Button>
          ))}
      </TableCell>
      <TableCell className="w-0">
        {!isExternal && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="secondary" className="h-auto w-max p-2">
                <MoreVertical size={16} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => handleEdit(account.original)}>
                {intl.formatMessage({
                  id: `common.edit`,
                  defaultMessage: 'Edit'
                })}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={() => {
                  onDelete(account.original.id!, account.original)
                }}
              >
                {intl.formatMessage({
                  id: `common.delete`,
                  defaultMessage: 'Delete'
                })}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </TableCell>
    </TableRow>
  )
}

export const AccountsDataTable: React.FC<AccountsTableProps> = ({
  accounts,
  table,
  onDelete,
  handleCreate,
  handleEdit,
  refetch,
  total,
  pagination,
  form,
  hasAssets
}) => {
  const intl = useIntl()
  const router = useRouter()

  return (
    <FormProvider {...form}>
      <div className="mb-4 flex justify-end">
        <PaginationLimitField control={form.control} />
      </div>

      <EntityDataTable.Root>
        {isNil(accounts?.items) || accounts?.items.length === 0 ? (
          <React.Fragment>
            {!hasAssets && (
              <div className="p-6">
                <Alert variant="warning" className="mb-6">
                  <AlertCircle className="h-4 w-4" />
                  <AlertTitle>
                    {intl.formatMessage({
                      id: 'accounts.alert.noAssets.title',
                      defaultMessage: 'No Asset Found'
                    })}
                  </AlertTitle>
                  <AlertDescription className="flex flex-col gap-2">
                    <span className="opacity-70">
                      {intl.formatMessage({
                        id: 'accounts.alert.noAssets.description',
                        defaultMessage:
                          'You need to create at least one asset before creating accounts.'
                      })}
                    </span>

                    <Button
                      variant="link"
                      className="w-fit p-0 text-yellow-800"
                      size="sm"
                      onClick={() => {
                        router.push('/assets?create=true')
                      }}
                    >
                      {intl.formatMessage({
                        id: 'accounts.alert.noAssets.createLink',
                        defaultMessage: 'Manage Assets'
                      })}
                    </Button>
                  </AlertDescription>
                </Alert>
              </div>
            )}

            <EmptyResource
              message={intl.formatMessage({
                id: 'ledgers.accounts.emptyResource',
                defaultMessage: "You haven't created any Accounts yet"
              })}
            >
              {!hasAssets ? (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <div className="inline-block">
                        <Button onClick={handleCreate} disabled>
                          {intl.formatMessage({
                            id: 'common.new.account',
                            defaultMessage: 'New Account'
                          })}
                        </Button>
                      </div>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs text-center">
                      {intl.formatMessage({
                        id: 'accounts.tooltip.noAssets',
                        defaultMessage:
                          'You need to create at least one asset before creating accounts.'
                      })}
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              ) : (
                <Button onClick={handleCreate}>
                  {intl.formatMessage({
                    id: 'common.new.account',
                    defaultMessage: 'New Account'
                  })}
                </Button>
              )}
            </EmptyResource>
          </React.Fragment>
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
                      id: 'entity.account.name',
                      defaultMessage: 'Account Name'
                    })}
                  </TableHead>
                  <TableHead className="text-center">
                    {intl.formatMessage({
                      id: 'common.assets',
                      defaultMessage: 'Assets'
                    })}
                  </TableHead>
                  <TableHead className="text-center">
                    {intl.formatMessage({
                      id: 'common.metadata',
                      defaultMessage: 'Metadata'
                    })}
                  </TableHead>
                  <TableHead className="text-center">
                    {intl.formatMessage({
                      id: 'common.portfolio',
                      defaultMessage: 'Portfolio'
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
                {table.getRowModel().rows.map((account) => (
                  <AccountRow
                    key={account.id}
                    account={account}
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
                number: accounts?.items.length,
                count: (
                  <span className="font-bold">{accounts?.items.length}</span>
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
