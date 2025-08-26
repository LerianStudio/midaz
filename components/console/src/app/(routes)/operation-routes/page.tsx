'use client'

import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { useOrganization } from '@lerianstudio/console-layout'
import React, { useState } from 'react'
import { useIntl } from 'react-intl'
import { AccountTypesSheet } from './operation-routes-sheet'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { AccountTypesDto } from '@/core/application/dto/account-types-dto'
import {
  useDeleteAccountType,
  useListAccountTypes
} from '@/client/account-types'
import { EntityBox } from '@/components/entity-box'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { Form } from '@/components/ui/form'
import { useQueryParams } from '@/hooks/use-query-params'
import { AccountTypesSkeleton } from './operation-routes-skeleton'
import { OperationRoutesDataTable } from './operation-routes-data-table'
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable
} from '@tanstack/react-table'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { toast } from '@/hooks/use-toast'
import { useListOperationRoutes } from '@/client/operation-routes'
import { OperationRoutesDto } from '@/core/application/dto/operation-routes-dto'

export default function Page() {
  const { currentOrganization, currentLedger } = useOrganization()
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = useState<any[]>([])

  const {
    handleCreate,
    handleEdit: handleEditOriginal,
    sheetProps
  } = useCreateUpdateSheet<OperationRoutesDto>({
    enableRouting: true
  })

  const [total, setTotal] = useState(1000000)

  const { form, searchValues, pagination } = useQueryParams({
    total,
    initialValues: {
      id: ''
    }
  })

  const {
    data: operationRoutesData,
    refetch: refetchOperationRoutes,
    isLoading: isOperationRoutesLoading
  } = useListOperationRoutes({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    query: searchValues as any
  })

  console.log('operationRoutesData', operationRoutesData)

  const accountTypesColumns = [
    {
      accessorKey: 'title',
      header: intl.formatMessage({
        id: 'account-types.field.name',
        defaultMessage: 'Name'
      })
    },
    {
      accessorKey: 'description',
      header: intl.formatMessage({
        id: 'account-types.field.description',
        defaultMessage: 'Description'
      })
    },
    {
      accessorKey: 'keyValue',
      header: intl.formatMessage({
        id: 'account-types.field.keyValue',
        defaultMessage: 'Key Value'
      })
    },
    {
      accessorKey: 'metadata',
      header: intl.formatMessage({
        id: 'account-types.field.metadata',
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
    data: operationRoutesData?.items ?? [],
    columns: accountTypesColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    state: {
      columnFilters
    }
  })

  // const {
  //   handleDialogOpen,
  //   dialogProps,
  //   handleDialogClose,
  //   data: selectedAccountType
  // } = useConfirmDialog<AccountTypesDto>({
  //   onConfirm: () => deleteAccountType(selectedAccountType)
  // })

  // const { mutate: deleteAccountType, isPending: deleteAccountTypePending } =
  //   useDeleteAccountType({
  //     organizationId: currentOrganization.id!,
  //     ledgerId: currentLedger.id,
  //     accountTypeId: selectedAccountType?.id || '',
  //     onSuccess: () => {
  //       handleDialogClose()
  //       refetchOperationRoutes()
  //       toast({
  //         description: intl.formatMessage(
  //           {
  //             id: 'success.accounts.delete',
  //             defaultMessage: '{accountName} account successfully deleted'
  //           },
  //           { accountName: selectedAccountType?.name! }
  //         ),
  //         variant: 'success'
  //       })
  //     }
  //   })

  const breadcrumbPaths = getBreadcrumbPaths([
    {
      name: currentOrganization.legalName
    },
    {
      name: currentLedger.name
    },
    {
      name: intl.formatMessage({
        id: `common.account-types`,
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
              id: 'common.account-types',
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
                id: 'common.new.account-type',
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

      {/* <ConfirmationDialog
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
      /> */}

      {/* <AccountTypesSheet
        ledgerId={currentLedger.id}
        onSuccess={() => refetchAccountTypes()}
        {...sheetProps}
      /> */}

      <Form {...form}>
        <EntityBox.Root>
          <div className="flex w-full justify-end">
            <PaginationLimitField control={form.control} />
          </div>
        </EntityBox.Root>

        {isOperationRoutesLoading && <AccountTypesSkeleton />}

        {!isOperationRoutesLoading && operationRoutesData && (
          <OperationRoutesDataTable
            operationRoutes={operationRoutesData}
            isLoading={isOperationRoutesLoading}
            handleCreate={handleCreate}
            handleEdit={handleEditOriginal}
            onDelete={() => {}}
            pagination={pagination}
            table={table}
            total={total}
          />
        )}
      </Form>
    </React.Fragment>
  )
}
