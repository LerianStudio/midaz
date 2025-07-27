'use client'

import React from 'react'
import { PageHeader } from '@/components/page-header'
import { useIntl } from 'react-intl'
import { Button } from '@/components/ui/button'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { LedgersDataTable } from './ledgers-data-table'
import { LedgersSheet } from './ledgers-sheet'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { useDeleteLedger, useListLedgers } from '@/client/ledgers'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { useOrganization } from '@/providers/organization-provider'
import { LedgersSkeleton } from './ledgers-skeleton'
import { useQueryParams } from '@/hooks/use-query-params'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { Breadcrumb } from '@/components/breadcrumb'
import { useToast } from '@/hooks/use-toast'

const Page = () => {
  const intl = useIntl()
  const [total, setTotal] = React.useState(0)
  const { currentOrganization, currentLedger, setLedger } = useOrganization()
  const { toast } = useToast()
  const { handleCreate, handleEdit, sheetProps } = useCreateUpdateSheet<any>({
    enableRouting: true
  })
  const { form, searchValues, pagination } = useQueryParams({ total })

  const {
    data: ledgers,
    refetch,
    isLoading
  } = useListLedgers({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    ...(searchValues as any)
  })

  React.useEffect(() => {
    if (!ledgers?.items) {
      setTotal(0)
      return
    }

    if (ledgers.items.length >= ledgers.limit) {
      setTotal(ledgers.limit + 1)
      return
    }

    setTotal(ledgers.items.length)
  }, [ledgers?.items, ledgers?.limit])

  const {
    handleDialogOpen,
    dialogProps,
    handleDialogClose,
    data: ledgerName
  } = useConfirmDialog({
    onConfirm: (id: string) => deleteMutate({ id })
  })

  const { mutate: deleteMutate, isPending: deletePending } = useDeleteLedger({
    organizationId: currentOrganization.id!,
    onSuccess: () => {
      handleDialogClose()

      const deletedLedgerId = ledgerName

      if (deletedLedgerId === currentLedger?.id) {
        const remainingLedgers =
          ledgers?.items?.filter((ledger) => ledger.id !== deletedLedgerId) ||
          []

        setLedger(
          remainingLedgers.length > 0 ? remainingLedgers[0] : ({} as any)
        )
      }

      toast({
        description: intl.formatMessage({
          id: 'success.ledgers.delete',
          defaultMessage: 'Ledger successfully deleted'
        }),
        variant: 'success'
      })
    }
  })

  const ledgersProps = {
    ledgers,
    handleDialogOpen,
    handleCreate,
    handleEdit,
    refetch,
    form,
    pagination,
    total
  }

  return (
    <React.Fragment>
      <Breadcrumb
        paths={getBreadcrumbPaths([
          {
            name: currentOrganization.legalName
          },
          {
            name: intl.formatMessage({
              id: `ledgers.title`,
              defaultMessage: 'Ledgers'
            })
          }
        ])}
      />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'ledgers.title',
              defaultMessage: 'Ledgers'
            })}
            subtitle={intl.formatMessage({
              id: 'ledgers.subtitle',
              defaultMessage:
                'Visualize and edit the Ledgers of your Organization.'
            })}
          />
          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'ledgers.helperTrigger.question',
                defaultMessage: 'What is a Ledger?'
              })}
            />
            <Button onClick={handleCreate} data-testid="new-ledger">
              {intl.formatMessage({
                id: 'ledgers.sheetCreate.title',
                defaultMessage: 'New Ledger'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>
        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'ledgers.helperTrigger.question',
            defaultMessage: 'What is a Ledger?'
          })}
          answer={intl.formatMessage({
            id: 'ledgers.helperTrigger.answer',
            defaultMessage:
              'Book with the record of all transactions and operations of the Organization.'
          })}
          seeMore={intl.formatMessage({
            id: 'ledgers.helperTrigger.seeMore',
            defaultMessage: 'Read the docs'
          })}
        />
      </PageHeader.Root>

      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'ledgers.deleteDialog.title',
          defaultMessage: 'Are you sure?'
        })}
        description={intl.formatMessage(
          {
            id: 'ledgers.deleteDialog.subtitle',
            defaultMessage:
              'This action is irreversible. This will deactivate your Ledger {ledgerName} forever'
          },
          { ledgerName: ledgerName as string }
        )}
        loading={deletePending}
        {...dialogProps}
      />

      <LedgersSheet onSuccess={refetch} {...sheetProps} />

      <div className="mt-10">
        {isLoading && <LedgersSkeleton />}

        {!isLoading && ledgers && <LedgersDataTable {...ledgersProps} />}
      </div>
    </React.Fragment>
  )
}

export default Page
