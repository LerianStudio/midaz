'use client'

import React, { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { useIntl } from 'react-intl'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useDeleteSegment, useListSegments } from '@/client/segments'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable
} from '@tanstack/react-table'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { useQueryParams } from '@/hooks/use-query-params'
import { SegmentsSheet } from './segments-sheet'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { Breadcrumb } from '@/components/breadcrumb'
import { PageHeader } from '@/components/page-header'
import { SegmentsDataTable } from './segments-data-table'
import { SegmentsSkeleton } from './segments-skeleton'
import { SegmentType } from '@/types/segment-type'

const Page = () => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const [columnFilters, setColumnFilters] = useState<any>([])

  const [total, setTotal] = useState(0)

  const { form, searchValues, pagination } = useQueryParams({
    total
  })

  const {
    data: segments,
    refetch,
    isLoading: isSegmentsLoading
  } = useListSegments({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    ...(searchValues as any)
  })

  useEffect(() => {
    if (!segments?.items) {
      setTotal(0)
      return
    }

    if (segments.items.length >= segments.limit) {
      setTotal(segments.limit + 1)
      return
    }

    setTotal(segments.items.length)
  }, [segments?.items, segments?.limit])

  const { mutate: deleteMutate, isPending: deletePending } = useDeleteSegment({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    onSuccess: () => {
      handleDialogClose()
      refetch()
    }
  })

  const { handleDialogOpen, dialogProps, handleDialogClose } = useConfirmDialog(
    {
      onConfirm: (id: string) => deleteMutate({ id })
    }
  )

  const { handleCreate, handleEdit, sheetProps } =
    useCreateUpdateSheet<SegmentType>({
      enableRouting: true
    })

  const table = useReactTable({
    data: segments?.items!,
    columns: [
      {
        accessorKey: 'name'
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
        id: `common.segments`,
        defaultMessage: 'Segments'
      })
    }
  ])

  const segmentsProps = {
    segments,
    table,
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
      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'common.segments',
              defaultMessage: 'Segments'
            })}
            subtitle={intl.formatMessage({
              id: 'segments.subtitle',
              defaultMessage: 'Manage the segments of this ledger.'
            })}
          />

          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'segments.helperTrigger.question',
                defaultMessage: 'What is a Segment?'
              })}
            />

            <Button onClick={handleCreate} data-testid="new-segment">
              {intl.formatMessage({
                id: 'segments.listingTemplate.addButton',
                defaultMessage: 'New Segment'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'segments.helperTrigger.question',
            defaultMessage: 'What is a Segment?'
          })}
          answer={intl.formatMessage({
            id: 'segments.helperTrigger.answer',
            defaultMessage:
              'Custom labels that allow grouping of accounts according to user-defined policies.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/docs/segments"
        />
      </PageHeader.Root>

      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'common.confirmDeletion',
          defaultMessage: 'Confirm Deletion'
        })}
        description={intl.formatMessage({
          id: 'segments.delete.description',
          defaultMessage:
            'You are about to permanently delete this segment. This action cannot be undone. Do you wish to continue?'
        })}
        loading={deletePending}
        {...dialogProps}
      />

      <SegmentsSheet
        ledgerId={currentLedger.id}
        onSuccess={refetch}
        {...sheetProps}
      />

      <div className="mt-10">
        {isSegmentsLoading && <SegmentsSkeleton />}

        {!isSegmentsLoading && segments && (
          <SegmentsDataTable {...segmentsProps} />
        )}
      </div>
    </React.Fragment>
  )
}

export default Page
