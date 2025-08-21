'use client'

import React, { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { useOrganization } from '@lerianstudio/console-layout'
import { useIntl } from 'react-intl'
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable
} from '@tanstack/react-table'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useAccountsWithPortfolios, useDeleteAccount } from '@/client/accounts'
import { AccountSheet } from './accounts-sheet'
import { AccountsDataTable } from './accounts-data-table'
import { useQueryParams } from '@/hooks/use-query-params'
import { PageHeader } from '@/components/page-header'
import { AccountsSkeleton } from './accounts-skeleton'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { Breadcrumb } from '@/components/breadcrumb'
import { useListAssets } from '@/client/assets'
import { useToast } from '@/hooks/use-toast'
import { AccountDto } from '@/core/application/dto/account-dto'
import { Form } from '@/components/ui/form'
import { EntityBox } from '@/components/entity-box'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { InputField } from '@/components/form'
import { useListAccountTypes } from '@/client/account-types'
import { useMidazConfig } from '@/hooks/use-midaz-config'
import { AlertCircle } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useRouter } from 'next/navigation'

const Page = () => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const [columnFilters, setColumnFilters] = useState<any>([])
  const { toast } = useToast()
  const router = useRouter()
  const [total, setTotal] = useState(1000000)

  const { form, searchValues, pagination } = useQueryParams({
    total,
    initialValues: {
      alias: '' 
    }
  })

  const {
    data: accountsData,
    refetch: refetchAccounts,
    isLoading: isAccountsLoading
  } = useAccountsWithPortfolios({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    query: searchValues as any
  })

  const accountsList: AccountDto[] = useMemo(() => {
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
  } = useConfirmDialog<AccountDto>({
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
  } = useCreateUpdateSheet<AccountDto>({
    enableRouting: true
  })

  const handleEdit = (account: AccountDto) => {
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

  const { data: accountTypesData } = useListAccountTypes({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    query: searchValues as any
  })

  const { isAccountTypeValidationEnabled: isValidationEnabled } = useMidazConfig()

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
                id: 'accounts.sheet.create.title',
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

        {isValidationEnabled && accountTypesData?.items.length === 0 && (
            <div className="p-6">
              <Alert variant="warning" className="mb-6">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>
                  {intl.formatMessage({
                    id: 'accounts.alert.noAccountType.title',
                    defaultMessage: 'Account Type Validation is Disabled'
                  })}
                </AlertTitle>
                <AlertDescription className="flex flex-col gap-2">
                  <span className="opacity-70">
                    {intl.formatMessage({
                      id: 'accounts.alert.noAccountType.description',
                      defaultMessage:
                        'Account Type Validation is disabled for this organization and ledger. You cannot create accounts.'
                    })}
                  </span>

                  <Button
                    variant="link"
                    className="w-fit p-0 text-yellow-800"
                    size="sm"
                    onClick={() => {
                      router.push('/account-types')
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
        searchValues={searchValues}
        accountTypesData={accountTypesData?.items}
        {...sheetProps}
      />

      <Form {...form}>
        <EntityBox.Root>
          <div>
            <InputField
              name="alias"
              placeholder={intl.formatMessage({
                id: 'accounts.search.placeholder',
                defaultMessage: 'Search by ID or Alias...'
              })}
              control={form.control}
            />
          </div>
          <EntityBox.Actions>
            <PaginationLimitField control={form.control} />
          </EntityBox.Actions>
        </EntityBox.Root>

        {isAccountsLoading && <AccountsSkeleton />}

        {!isAccountsLoading && accountsList && !isAssetsLoading && (
          <AccountsDataTable
            accounts={{ items: accountsList }}
            isLoading={isAccountsLoading}
            table={table}
            handleCreate={handleCreate}
            handleEdit={handleEdit}
            onDelete={handleDialogOpen}
            _refetch={refetchAccounts}
            total={total}
            pagination={pagination}
            hasAssets={hasAssets || false}
          />
        )}
      </Form>
    </React.Fragment>
  )
}

export default Page
