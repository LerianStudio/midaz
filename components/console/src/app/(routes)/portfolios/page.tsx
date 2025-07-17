'use client'

import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { PortfolioDto } from '@/core/application/dto/portfolio-dto'
import {
  useDeletePortfolio,
  usePortfoliosWithAccounts
} from '@/client/portfolios'
import { useOrganization } from '@/providers/organization-provider'
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
import { useToast } from '@/hooks/use-toast'
import { Form } from '@/components/ui/form'
import { EntityBox } from '@/components/entity-box'
import { InputField } from '@/components/form'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'

const Page = () => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const { toast } = useToast()
  const [total, setTotal] = useState(0)
  const { form, searchValues, pagination } = useQueryParams({
    total,
    initialValues: {
      id: ''
    }
  })

  const {
    data: portfolios,
    refetch,
    isLoading: isLoadingPortfolios
  } = usePortfoliosWithAccounts({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    query: searchValues as any
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

  const { mutate: deletePortfolio, isPending: deletePending } =
    useDeletePortfolio({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      onSuccess: () => {
        handleDialogClose()
        refetch()
        toast({
          description: intl.formatMessage({
            id: 'success.portfolios.delete',
            defaultMessage: 'Portfolio successfully deleted'
          }),
          variant: 'success'
        })
      }
    })

  const { handleDialogOpen, dialogProps, handleDialogClose } = useConfirmDialog(
    {
      onConfirm: (id: string) => deletePortfolio({ id })
    }
  )

  const { handleCreate, handleEdit, sheetProps } =
    useCreateUpdateSheet<PortfolioDto>({
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
              'Groups of accounts assembled for organizational and operational purposes.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/docs/portfolios"
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

      <Form {...form}>
        <EntityBox.Root>
          <div>
            <InputField
              name="id"
              placeholder={intl.formatMessage({
                id: 'common.searchById',
                defaultMessage: 'Search by ID...'
              })}
              control={form.control}
            />
          </div>
          <EntityBox.Actions>
            <PaginationLimitField control={form.control} />
          </EntityBox.Actions>
        </EntityBox.Root>

        {isLoadingPortfolios && <PortfoliosSkeleton />}

        {!isLoadingPortfolios && (
          <PortfoliosDataTable {...portfoliosTableProps} />
        )}
      </Form>
    </React.Fragment>
  )
}

export default Page
