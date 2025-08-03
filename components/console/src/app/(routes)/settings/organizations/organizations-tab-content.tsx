'use client'

import { Button } from '@/components/ui/button'
import { MoreVertical } from 'lucide-react'
import { useRouter } from 'next/navigation'
import React from 'react'
import { useIntl } from 'react-intl'
import { EmptyResource } from '@/components/empty-resource'
import { EntityBox } from '@/components/entity-box'
import {
  useDeleteOrganization,
  useListOrganizations
} from '@/client/organizations'
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { EntityDataTable } from '@/components/entity-data-table'
import { Skeleton } from '@/components/ui/skeleton'
import { OrganizationDto } from '@/core/application/dto/organization-dto'
import { useOrganization } from '@lerianstudio/console-layout'
import { IdTableCell } from '@/components/table/id-table-cell'
import { InputField } from '@/components/form'
import { Pagination } from '@/components/pagination'
import { useQueryParams } from '@/hooks/use-query-params'
import { Form } from '@/components/ui/form'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'

export const OrganizationsTabContent = () => {
  const intl = useIntl()
  const { currentOrganization, setOrganization } = useOrganization()
  const router = useRouter()

  const [total, setTotal] = React.useState(0)

  const { form, searchValues, pagination } = useQueryParams({
    total,
    initialValues: {
      id: ''
    }
  })

  const { data, isLoading } = useListOrganizations({
    query: searchValues as any
  })

  // Update total count when data is received - following pagination pattern from other components
  React.useEffect(() => {
    if (!data?.items) {
      setTotal(0)
      return
    }

    if (data.items.length >= data.limit) {
      setTotal(data.limit + 1)
      return
    }

    setTotal(data.items.length)
  }, [data?.items, data?.limit])

  const { mutate: deleteOrganization, isPending: deletePending } =
    useDeleteOrganization({
      onSuccess: () => {
        handleDialogClose()
      }
    })

  const { handleDialogOpen, dialogProps, handleDialogClose } = useConfirmDialog(
    {
      onConfirm: (id: string) => deleteOrganization({ id })
    }
  )

  const handleEdit = (organization: OrganizationDto) => {
    router.push(`/settings/organizations/${organization.id}`)
  }

  const handleCreateOrganization = () => {
    router.push(`/settings/organizations/new-organization`)
  }

  return (
    <div>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'common.confirmDeletion',
          defaultMessage: 'Confirm Deletion'
        })}
        description={intl.formatMessage({
          id: 'organizations.delete.description',
          defaultMessage: 'You will delete an organization'
        })}
        loading={deletePending}
        {...dialogProps}
      />

      <Form {...form}>
        <EntityBox.Root>
          <EntityBox.Header
            title={intl.formatMessage({
              id: 'organizations.title',
              defaultMessage: 'Organizations'
            })}
            subtitle={intl.formatMessage({
              id: 'organizations.subtitle',
              defaultMessage: 'View and manage Organizations.'
            })}
            tooltip={intl.formatMessage({
              id: 'organizations.tooltip',
              defaultMessage:
                'Organizations is the top-level entity in Midaz, representing a financial institution such as a bank or fintech'
            })}
            tooltipWidth="655px"
          />

          <div className="flex grow flex-row items-center justify-center">
            <InputField
              className="w-[252px]"
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
            <Button onClick={handleCreateOrganization}>
              {intl.formatMessage({
                id: 'common.create',
                defaultMessage: 'Create'
              })}
            </Button>
          </EntityBox.Actions>
        </EntityBox.Root>

        {data?.items && data.items.length === 0 && (
          <EmptyResource
            message={intl.formatMessage({
              id: 'organizations.emptyResource',
              defaultMessage: "You haven't created any Organization yet"
            })}
          >
            <Button variant="outline" onClick={handleCreateOrganization}>
              {intl.formatMessage({
                id: 'common.create',
                defaultMessage: 'Create'
              })}
            </Button>
          </EmptyResource>
        )}

        {isLoading && (
          <Skeleton className="mt-4 h-[390px] w-full bg-zinc-200" />
        )}

        {!isLoading && data?.items && data.items.length > 0 && (
          <EntityDataTable.Root>
            <TableContainer>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>
                      {intl.formatMessage({
                        id: 'organizations.field.legalName',
                        defaultMessage: 'Legal Name'
                      })}
                    </TableHead>
                    <TableHead>
                      {intl.formatMessage({
                        id: 'common.id',
                        defaultMessage: 'ID'
                      })}
                    </TableHead>
                    <TableHead>
                      {intl.formatMessage({
                        id: 'organizations.field.doingBusinessAs',
                        defaultMessage: 'Doing Business As'
                      })}
                    </TableHead>
                    <TableHead>
                      {intl.formatMessage({
                        id: 'organizations.field.legalDocument',
                        defaultMessage: 'Legal Document'
                      })}
                    </TableHead>
                    <TableHead align="center">
                      {intl.formatMessage({
                        id: 'common.actions',
                        defaultMessage: 'Actions'
                      })}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.items.map((organization) => (
                    <TableRow key={organization.id}>
                      <TableCell>{organization.legalName}</TableCell>
                      <IdTableCell id={organization.id} />
                      <TableCell>{organization.doingBusinessAs}</TableCell>
                      <TableCell>{organization.legalDocument}</TableCell>
                      <TableCell align="center">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="secondary"
                              className="h-auto w-max p-2"
                            >
                              <MoreVertical size={16} onClick={() => {}} />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem
                              onClick={() => handleEdit(organization)}
                            >
                              {intl.formatMessage({
                                id: 'common.edit',
                                defaultMessage: 'Edit'
                              })}
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              onClick={() =>
                                handleDialogOpen(organization.id!)
                              }
                            >
                              {intl.formatMessage({
                                id: 'common.delete',
                                defaultMessage: 'Delete'
                              })}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>

            <EntityDataTable.Footer className="flex items-center justify-between py-4">
              <EntityDataTable.FooterText>
                {intl.formatMessage(
                  {
                    id: 'organizations.showing',
                    defaultMessage:
                      '{number, plural, =0 {No organizations found} one {Showing {count} organization} other {Showing {count} organizations}}.'
                  },
                  {
                    number: data?.items?.length,
                    count: (
                      <span className="font-bold">
                        {data?.items?.length}
                      </span>
                    )
                  }
                )}
              </EntityDataTable.FooterText>
              <Pagination total={total} {...pagination} />
            </EntityDataTable.Footer>
          </EntityDataTable.Root>
        )}
      </Form>
    </div>
  )
}
