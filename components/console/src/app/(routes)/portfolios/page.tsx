'use client'

import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { PortfolioResponseDto } from '@/core/application/dto/portfolios-dto'
import {
  useDeletePortfolio,
  usePortfoliosWithAccounts
} from '@/client/portfolios'
import { useOrganization } from '@/context/organization-provider/organization-provider-client'
import { useIntl } from 'react-intl'
import React, { useEffect, useState } from 'react'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useQueryParams } from '@/hooks/use-query-params'
import { PortfolioSheet } from './portfolios-sheet'
import { PortfoliosSkeleton } from './portfolios-skeleton'
import { PortfoliosDataTable } from './portfolios-data-table'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { Breadcrumb } from '@/components/breadcrumb'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import useCustomToast from '@/hooks/use-custom-toast'
import { useRouter } from 'next/navigation'

const Page = () => {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization, currentLedger } = useOrganization()
  const { showSuccess, showError } = useCustomToast()
  const [total, setTotal] = useState(0)
  const { form, searchValues, pagination } = useQueryParams({
    total
  })

  const {
    data: portfolios,
    refetch,
    isLoading: isLoadingPortfolios
  } = usePortfoliosWithAccounts({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    ...(searchValues as any)
  })

  useEffect(() => {
    if (!portfolios?.items) {
      setTotal(0)
      return
    }

    if (portfolios.items.length >= portfolios.limit) {
      setTotal(portfolios.limit + 1)
      return
    }

    setTotal(portfolios.items.length)
  }, [portfolios?.items, portfolios?.limit])

  useEffect(() => {
    if (!currentLedger?.id) {
      router.replace('/ledgers')
    }
  }, [currentLedger, router])

  const { mutate: deletePortfolio, isPending: deletePending } =
    useDeletePortfolio({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      onSuccess: () => {
        handleDialogClose()
        refetch()
        showSuccess(
          intl.formatMessage({
            id: 'portfolios.toast.delete.success',
            defaultMessage: 'Portfolio successfully deleted'
          })
        )
      },
      onError: () => {
        handleDialogClose()
        showError(
          intl.formatMessage({
            id: 'portfolios.toast.delete.error',
            defaultMessage: 'Error deleting Portfolio'
          })
        )
      }
    })

  const { handleDialogOpen, dialogProps, handleDialogClose } = useConfirmDialog(
    {
      onConfirm: (id: string) => deletePortfolio({ id })
    }
  )

  const { handleCreate, handleEdit, sheetProps } =
    useCreateUpdateSheet<PortfolioResponseDto>({
      enableRouting: true
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
        id: 'common.portfolios',
        defaultMessage: 'Portfolios'
      })
    }
  ])

  const portfoliosTableProps = {
    portfolios,
    handleCreate,
    handleDialogOpen,
    handleEdit,
    form,
    pagination,
    total
  }

  return (
    <React.Fragment>
      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'common.portfolios',
              defaultMessage: 'Portfolios'
            })}
            subtitle={intl.formatMessage({
              id: 'portfolios.subtitle',
              defaultMessage: 'Manage portfolios on this ledger.'
            })}
          />

          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'portfolios.helperTrigger.question',
                defaultMessage: 'What is a Portfolio?'
              })}
            />

            <Button onClick={handleCreate} data-testid="new-portfolio">
              {intl.formatMessage({
                id: 'portfolios.listingTemplate.addButton',
                defaultMessage: 'New Portfolio'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'portfolios.helperTrigger.question',
            defaultMessage: 'What is a Portfolio?'
          })}
          answer={intl.formatMessage({
            id: 'portfolios.helperTrigger.answer',
            defaultMessage:
              'Book with the record of all transactions and operations of the Organization.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
        />
      </PageHeader.Root>

      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'ledgers.portfolio.deleteDialog.title',
          defaultMessage: 'Are you sure?'
        })}
        description={intl.formatMessage({
          id: 'ledgers.portfolio.deleteDialog.description',
          defaultMessage: 'You will delete a portfolio'
        })}
        loading={deletePending}
        {...dialogProps}
      />

      <PortfolioSheet onSuccess={refetch} {...sheetProps} />

      <div className="mt-10">
        {isLoadingPortfolios && <PortfoliosSkeleton />}

        {!isLoadingPortfolios && (
          <PortfoliosDataTable {...portfoliosTableProps} />
        )}
      </div>
    </React.Fragment>
  )
}

export default Page
