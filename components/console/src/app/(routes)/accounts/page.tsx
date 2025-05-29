'use client'

import React, { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { useIntl } from 'react-intl'
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable
} from '@tanstack/react-table'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useAccountsWithPortfolios, useDeleteAccount } from '@/client/accounts'
import { AccountType } from '@/types/accounts-type'
import { AccountSheet } from './accounts-sheet'
import { AccountsDataTable } from './accounts-data-table'
import { useQueryParams } from '@/hooks/use-query-params'
import { PageHeader } from '@/components/page-header'
import { AccountsSkeleton } from './accounts-skeleton'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { Breadcrumb } from '@/components/breadcrumb'
import { useListAssets } from '@/client/assets'
import { useToast } from '@/hooks/use-toast'

const Page = () => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const [columnFilters, setColumnFilters] = useState<any>([])
  const { toast } = useToast()

  const [total, setTotal] = useState(0)

  const { form, searchValues, pagination } = useQueryParams({
    total
  })

  const {
    data: accountsData,
    refetch: refetchAccounts,
    isLoading: isAccountsLoading
  } = useAccountsWithPortfolios({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    ...(searchValues as any)
  })

  useEffect(() => {
    if (!accountsData?.items) {
      setTotal(0)
      return
    }

    if (accountsData.items.length >= accountsData.limit) {
      setTotal(accountsData.limit + 1)
      return
    }

    setTotal(accountsData.items.length)
  }, [accountsData?.items, accountsData?.limit])

  const accountsList: AccountType[] = useMemo(() => {
    return (
      accountsData?.items.map((account: any) => ({
        ...account,
        assetCode: account.assetCode,
        parentAccountId: account.parentAccountId,
        segmentId: account.segmentId,
        metadata: account.metadata,
        portfolioId: account.portfolio?.id,
        portfolioName: account.portfolio?.name
      })) || []
    )
  }, [accountsData])

  const {
    handleDialogOpen,
    dialogProps,
    handleDialogClose,
    data: selectedAccount
  } = useConfirmDialog<AccountType>({
    onConfirm: () => deleteAccount(selectedAccount)
  })

  const { mutate: deleteAccount, isPending: deleteAccountPending } =
    useDeleteAccount({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      accountId: selectedAccount?.id || '',
      onSuccess: () => {
        handleDialogClose()
        refetchAccounts()
        toast({
          description: intl.formatMessage(
            {
              id: 'success.accounts.delete',
              defaultMessage: '{accountName} account successfully deleted'
            },
            { accountName: selectedAccount?.name! }
          ),
          variant: 'success'
        })
      }
    })

  const { data: assetsData, isLoading: isAssetsLoading } = useListAssets({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    limit: 1,
    page: 1
  })

  const hasAssets = assetsData?.items && assetsData.items.length > 0

  const {
    handleCreate,
    handleEdit: handleEditOriginal,
    sheetProps
  } = useCreateUpdateSheet<AccountType>({
    enableRouting: true
  })

  const handleEdit = (account: AccountType) => {
    handleEditOriginal(account)
  }

  const table = useReactTable({
    data: accountsList,
    columns: [
      { accessorKey: 'id', header: 'ID' },
      {
        accessorKey: 'name',
        header: intl.formatMessage({
          id: 'accounts.field.name',
          defaultMessage: 'Account Name'
        })
      },
      {
        accessorKey: 'alias',
        header: intl.formatMessage({
          id: 'accounts.field.alias',
          defaultMessage: 'Account Alias'
        })
      },
      {
        accessorKey: 'assetCode',
        header: intl.formatMessage({
          id: 'common.assets',
          defaultMessage: 'Assets'
        }),
        cell: (info) => info.getValue() || '-'
      },
      {
        accessorKey: 'metadata',
        header: intl.formatMessage({
          id: 'common.metadata',
          defaultMessage: 'Metadata'
        }),
        cell: (info) => {
          const metadata = info.getValue() || {}
          const count = Object.keys(metadata).length
          return count > 0
            ? `${count} ${intl.formatMessage({ id: 'common.records', defaultMessage: 'records' })}`
            : '-'
        }
      },
      {
        accessorKey: 'portfolio.name',
        header: intl.formatMessage({
          id: 'common.portfolio',
          defaultMessage: 'Portfolio'
        }),
        cell: (info) => info.getValue() || '-'
      },
      {
        accessorKey: 'actions',
        header: intl.formatMessage({
          id: 'common.actions',
          defaultMessage: 'Actions'
        }),
        cell: () => null
      }
    ],
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    state: {
      columnFilters
    }
  })

  const breadcrumbPaths = getBreadcrumbPaths([
    {
      name: currentOrganization.legalName
    },
    {
      name: currentLedger.name
    },
    {
      name: intl.formatMessage({
        id: `common.accounts`,
        defaultMessage: 'Accounts'
      })
    }
  ])

  return (
    <React.Fragment>
      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'common.accounts',
              defaultMessage: 'Accounts'
            })}
            subtitle={intl.formatMessage({
              id: 'accounts.subtitle',
              defaultMessage: 'Manage the accounts of this ledger.'
            })}
          />
          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'accounts.helperTrigger.question',
                defaultMessage: 'What is an Account?'
              })}
            />

            <Button
              onClick={handleCreate}
              data-testid="new-account"
              disabled={!hasAssets}
            >
              {intl.formatMessage({
                id: 'accounts.listingTemplate.addButton',
                defaultMessage: 'New Account'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'accounts.helperTrigger.question',
            defaultMessage: 'What is an Account?'
          })}
          answer={intl.formatMessage({
            id: 'accounts.helperTrigger.answer',
            defaultMessage:
              'Accounts linked to specific assets, used to record balances and financial movements.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/docs/accounts"
        />
      </PageHeader.Root>

      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'common.confirmDeletion',
          defaultMessage: 'Confirm Deletion'
        })}
        description={intl.formatMessage({
          id: 'accounts.delete.description',
          defaultMessage: 'You will delete an account'
        })}
        loading={deleteAccountPending}
        {...dialogProps}
      />

      <AccountSheet
        ledgerId={currentLedger.id}
        onSuccess={refetchAccounts}
        {...sheetProps}
      />

      <div className="mt-10">
        {isAccountsLoading && <AccountsSkeleton />}

        {!isAccountsLoading && accountsList && !isAssetsLoading && (
          <AccountsDataTable
            accounts={{ items: accountsList }}
            isLoading={isAccountsLoading}
            table={table}
            handleCreate={handleCreate}
            handleEdit={handleEdit}
            onDelete={handleDialogOpen}
            refetch={refetchAccounts}
            total={total}
            pagination={pagination}
            form={form}
            hasAssets={hasAssets || false}
          />
        )}
      </div>
    </React.Fragment>
  )
}

export default Page
