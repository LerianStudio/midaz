'use client'

import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { useOrganization } from '@lerianstudio/console-layout'
import React, { useState } from 'react'
import { useIntl } from 'react-intl'
import { OperationRoutesSheet } from './operation-routes-sheet'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { EntityBox } from '@/components/entity-box'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { Form } from '@/components/ui/form'
import { useQueryParams } from '@/hooks/use-query-params'
import { OperationRoutesSkeleton } from './operation-routes-skeleton'
import { OperationRoutesDataTable } from './operation-routes-data-table'
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable
} from '@tanstack/react-table'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { toast } from '@/hooks/use-toast'
import { useDeleteOperationRoute, useListOperationRoutes } from '@/client/operation-routes'
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

  const operationRoutesColumns = [
    {
      accessorKey: 'title',
      header: intl.formatMessage({
        id: 'operation-routes.field.title',
        defaultMessage: 'Title'
      })
    },
    {
      accessorKey: 'description',
      header: intl.formatMessage({
        id: 'operation-routes.field.description',
        defaultMessage: 'Description'
      })
    },
    {
      accessorKey: 'operationType',
      header: intl.formatMessage({
        id: 'operation-routes.field.operationType',
        defaultMessage: 'Operation Type'
      })
    },
    {
      accessorKey: 'account',
      header: intl.formatMessage({
        id: 'operation-routes.field.account',
        defaultMessage: 'Account Rule'
      }),
      cell: ({ getValue }: any) => {
        const account = getValue()
        if (!account) return '-'
        const validIfDisplay = Array.isArray(account.validIf)
          ? account.validIf.join(', ')
          : account.validIf
        return `${account.ruleType}: ${validIfDisplay}`
      }
    },
    {
      accessorKey: 'metadata',
      header: intl.formatMessage({
        id: 'common.metadata',
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
    columns: operationRoutesColumns,
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
    data: selectedOperationRoute
  } = useConfirmDialog<OperationRoutesDto>({
    onConfirm: () => deleteOperationRoute(selectedOperationRoute)
  })

  const { mutate: deleteOperationRoute, isPending: deleteOperationRoutePending } =
    useDeleteOperationRoute({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      operationRouteId: selectedOperationRoute?.id || '',
      onSuccess: () => {
        handleDialogClose()
        refetchOperationRoutes()
        toast({
          description: intl.formatMessage(
            {
              id: 'success.operation-routes.delete',
              defaultMessage: '{operationRouteTitle} operation route successfully deleted'
            },
            { operationRouteTitle: selectedOperationRoute?.title! }
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
        id: `common.operation-routes`,
        defaultMessage: 'Operation Routes'
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
              id: 'common.operation-routes',
              defaultMessage: 'Operation Routes'
            })}
            subtitle={intl.formatMessage({
              id: 'operation-routes.subtitle',
              defaultMessage: 'Manage the operation routes of this ledger.'
            })}
          />
          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'operation-routes.helperTrigger.question',
                defaultMessage: 'What is an Operation Route?'
              })}
            />

            <Button onClick={handleCreate} data-testid="new-operation-route">
              {intl.formatMessage({
                id: 'common.new.operation-route',
                defaultMessage: 'New Operation Route'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'operation-routes.helperTrigger.question',
            defaultMessage: 'What is an Operation Route?'
          })}
          answer={intl.formatMessage({
            id: 'operation-routes.helperTrigger.answer',
            defaultMessage:
              'Operation routes define rules for validating accounts during financial operations. They specify criteria that accounts must meet to participate in transactions.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/reference/create-an-operation-route"
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
        loading={deleteOperationRoutePending}
        {...dialogProps}
      />

      <OperationRoutesSheet
        ledgerId={currentLedger.id}
        onSuccess={() => refetchOperationRoutes()}
        {...sheetProps}
      />

      <Form {...form}>
        <EntityBox.Root>
          <div className="flex w-full justify-end">
            <PaginationLimitField control={form.control} />
          </div>
        </EntityBox.Root>

        {isOperationRoutesLoading && <OperationRoutesSkeleton />}

        {!isOperationRoutesLoading && operationRoutesData && (
          <OperationRoutesDataTable
            operationRoutes={operationRoutesData}
            isLoading={isOperationRoutesLoading}
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
