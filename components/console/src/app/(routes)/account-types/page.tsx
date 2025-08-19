'use client'

import { Breadcrumb } from "@/components/breadcrumb"
import { getBreadcrumbPaths } from "@/components/breadcrumb/get-breadcrumb-paths"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { useOrganization } from "@lerianstudio/console-layout"
import React, { useState } from "react"
import { useIntl } from "react-intl"
import { AccountTypesSheet } from "./account-types-sheet"
import { useCreateUpdateSheet } from "@/components/sheet/use-create-update-sheet"
import { AccountTypesDto } from "@/core/application/dto/account-types-dto"
import { useAccountType, useDeleteAccountType, useListAccountTypes } from "@/client/account-types"
import { InputField } from "@/components/form"
import { EntityBox } from "@/components/entity-box"
import { PaginationLimitField } from "@/components/form/pagination-limit-field"
import { Form } from "@/components/ui/form"
import { useQueryParams } from "@/hooks/use-query-params"
import { AccountTypesSkeleton } from "./account-types-skeleton"
import { AccountTypesDataTable } from "./account-types-data-table"
import { getCoreRowModel, getFilteredRowModel, useReactTable } from "@tanstack/react-table"
import ConfirmationDialog from "@/components/confirmation-dialog"
import { AccountDto } from "@/core/application/dto/account-dto"
import { useConfirmDialog } from "@/components/confirmation-dialog/use-confirm-dialog"
import { toast } from "@/hooks/use-toast"

export default function Page() {
  const { currentOrganization, currentLedger } = useOrganization()
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = useState<any[]>([])


  const {
    handleCreate,
    handleEdit: handleEditOriginal,
    sheetProps
  } = useCreateUpdateSheet<AccountTypesDto>({
    enableRouting: true
  })  

  const [total, setTotal] = useState(1000000)

  const { form, searchValues, pagination } = useQueryParams({
    total,
    initialValues: {
      alias: ''
    }
  })

  const {
    data: accountTypesData,
    refetch: refetchAccountTypes,
    isLoading: isAccountTypesLoading
  } = useListAccountTypes({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    query: searchValues as any
  })

  const accountTypesColumns = [
    {
      accessorKey: 'name',
      header: 'Name'
    },
    {
      accessorKey: 'description',
      header: 'Description'
    },
    {
      accessorKey: 'keyValue',
      header: 'Key Value'
    },
    {
      accessorKey: 'metadata',
      header: 'Metadata'
    }
  ]

  const table = useReactTable({
    data: accountTypesData?.items ?? [],
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

  console.log(selectedAccountType)

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

            <Button
              onClick={handleCreate}
              data-testid="new-account-type"
            >
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


      <Form {...form}>
        <EntityBox.Root>
          <div>
            <InputField
              name="name"
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

        {isAccountTypesLoading && <AccountTypesSkeleton />}

        {!isAccountTypesLoading && accountTypesData && (
          <AccountTypesDataTable
            accountTypes={accountTypesData}
            isLoading={isAccountTypesLoading}
            handleCreate={handleCreate}
            handleEdit={handleEditOriginal}
            onDelete={handleDialogOpen}
            pagination={pagination}
            table={table}
            total={total}
          />
        )}
      </Form>
    </React.Fragment>
  )
}