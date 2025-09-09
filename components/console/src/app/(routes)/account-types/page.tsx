'use client'

import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { useOrganization } from '@lerianstudio/console-layout'
import React, { useState } from 'react'
import { useIntl } from 'react-intl'
import { AccountTypesSheet } from './account-types-sheet'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { AccountTypesDto } from '@/core/application/dto/account-types-dto'
import { useDeleteAccountType } from '@/client/account-types'
import { EntityBox } from '@/components/entity-box'
import { AccountTypesSkeleton } from './account-types-skeleton'
import { AccountTypesDataTable } from './account-types-data-table'
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable
} from '@tanstack/react-table'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { toast } from '@/hooks/use-toast'
import { useAccountTypesCursor } from '@/hooks/use-account-types-cursor'
import { PageCounter } from '@/components/page-counter'

export default function Page() {
  const { currentOrganization, currentLedger } = useOrganization()
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = useState<any[]>([])
  const [searchId, setSearchId] = useState('')

  const {
    handleCreate,
    handleEdit: handleEditOriginal,
    sheetProps
  } = useCreateUpdateSheet<AccountTypesDto>({
    enableRouting: true
  })

  const {
    accountTypes,
    isLoading: isAccountTypesLoading,
    isEmpty,
    hasNext,
    hasPrev,
    nextPage,
    previousPage,
    goToFirstPage,
    setSortOrder,
    sortOrder,
    setLimit,
    limit,
    refetch: refetchAccountTypes
  } = useAccountTypesCursor({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    searchParams: {
      id: searchId || undefined,
      sortBy: 'createdAt'
    },
    limit: 10,
    sortOrder: 'desc'
  })

  const accountTypesColumns = [
    {
      accessorKey: 'name',
      header: intl.formatMessage({
        id: 'accountTypes.field.name',
        defaultMessage: 'Account Type Name'
      })
    },
    {
      accessorKey: 'description',
      header: intl.formatMessage({
        id: 'accountTypes.field.description',
        defaultMessage: 'Description'
      })
    },
    {
      accessorKey: 'keyValue',
      header: intl.formatMessage({
        id: 'accountTypes.field.keyValue',
        defaultMessage: 'Key Value'
      })
    },
    {
      accessorKey: 'metadata',
      header: intl.formatMessage({
        id: 'accountTypes.field.metadata',
        defaultMessage: 'Metadata'
      })
    },
    {
      accessorKey: 'actions',
      header: intl.formatMessage({
        id: 'common.actions',
        defaultMessage: 'Actions'
      })
    }
  ]

  const table = useReactTable({
    data: accountTypes ?? [],
    columns: accountTypesColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    state: {
      columnFilters
    }
  })

  const {
    handleDialogOpen,
    dialogProps,
    handleDialogClose,
    data: selectedAccountType
  } = useConfirmDialog<AccountTypesDto>({
    onConfirm: () => deleteAccountType(selectedAccountType)
  })

  const { mutate: deleteAccountType, isPending: deleteAccountTypePending } =
    useDeleteAccountType({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      accountTypeId: selectedAccountType?.id || '',
      onSuccess: () => {
        handleDialogClose()
        refetchAccountTypes()
        toast({
          description: intl.formatMessage(
            {
              id: 'success.accounts.delete',
              defaultMessage: '{accountName} account successfully deleted'
            },
            { accountName: selectedAccountType?.name! }
          ),
          variant: 'success'
        })
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
        id: `common.accountTypes`,
        defaultMessage: 'Account Types'
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
              id: 'common.accountTypes',
              defaultMessage: 'Account Types'
            })}
            subtitle={intl.formatMessage({
              id: 'account-type.subtitle',
              defaultMessage: 'Manage the account types of this ledger.'
            })}
          />
          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'account-type.helperTrigger.question',
                defaultMessage: 'What is an Account Type?'
              })}
            />

            <Button onClick={handleCreate} data-testid="new-account-type">
              {intl.formatMessage({
                id: 'common.new.accountType',
                defaultMessage: 'New Account Type'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'account-type.helperTrigger.question',
            defaultMessage: 'What is an Account Type?'
          })}
          answer={intl.formatMessage({
            id: 'account-type.helperTrigger.answer',
            defaultMessage:
              'Account types are used to categorize accounts. They are used to group accounts with similar characteristics.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/reference/create-an-account-type"
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
        loading={deleteAccountTypePending}
        {...dialogProps}
      />

      <AccountTypesSheet
        ledgerId={currentLedger.id}
        onSuccess={() => refetchAccountTypes()}
        {...sheetProps}
      />

      <EntityBox.Root>
        <PageCounter
          limit={limit}
          setLimit={setLimit}
          limitValues={[5, 10, 20, 50]}
        />
      </EntityBox.Root>

      {isAccountTypesLoading && <AccountTypesSkeleton />}

      {!isAccountTypesLoading && (
        <>
          <AccountTypesDataTable
            accountTypes={{
              items: accountTypes,
              limit: limit,
              nextCursor: undefined,
              prevCursor: undefined
            }}
            isLoading={isAccountTypesLoading}
            handleCreate={handleCreate}
            handleEdit={handleEditOriginal}
            onDelete={handleDialogOpen}
            table={table}
            useCursorPagination={true}
            cursorPaginationControls={{
              hasNext,
              hasPrev,
              nextPage,
              previousPage,
              goToFirstPage
            }}
          />
        </>
      )}
    </React.Fragment>
  )
}
