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
import { IdTableCell } from '@/components/table/id-table-cell'
import { InputField } from '@/components/form'
import { Pagination } from '@/components/pagination'
import { useQueryParams } from '@/hooks/use-query-params'
import { Form } from '@/components/ui/form'
import { useToast } from '@/hooks/use-toast'
import { PaginationLimitField } from '@/components/form/pagination-limit-field'
import { useOrganization } from '@lerianstudio/console-layout'

export const OrganizationsTabContent = () => {
  const intl = useIntl()
  const router = useRouter()
  const { toast } = useToast()
  const { currentOrganization, setOrganization } = useOrganization()

  const [total, setTotal] = React.useState(1000000)

  const { form, searchValues, pagination } = useQueryParams({
    total,
    initialValues: {
      id: ''
    }
  })

  const { data, isLoading, refetch } = useListOrganizations({
    query: searchValues as any
  })

  const { mutate: deleteOrganization, isPending: deletePending } =
    useDeleteOrganization({
      onSuccess: () => {
        handleDialogClose()
        refetch()
        toast({
          description: intl.formatMessage({
            id: 'organizations.toast.delete.success',
            defaultMessage: 'Organization successfully deleted'
          }),
          variant: 'success'
        })
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
    <div data-testid="organizations-tab-content">
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
              data-testid="organizations-search-input"
            />
          </div>
          <EntityBox.Actions>
            <PaginationLimitField control={form.control} />
            <Button
              onClick={handleCreateOrganization}
              data-testid="organizations-create-button"
            >
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
            data-testid="organizations-empty-state"
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
          <Skeleton
            className="mt-4 h-[390px] w-full bg-zinc-200"
            data-testid="organizations-loading"
          />
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
                    <TableRow
                      key={organization.id}
                      data-testid={`organization-row-${organization.id}`}
                    >
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
                              data-testid={`organization-menu-trigger-${organization.id}`}
                            >
                              <MoreVertical size={16} onClick={() => {}} />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem
                              onClick={() => handleEdit(organization)}
                              data-testid={`organization-edit-${organization.id}`}
                            >
                              {intl.formatMessage({
                                id: 'common.edit',
                                defaultMessage: 'Edit'
                              })}
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            {currentOrganization.id !== organization.id && (
                              <>
                                <DropdownMenuItem
                                  onClick={() => setOrganization(organization)}
                                >
                                  {intl.formatMessage({
                                    id: `organizations.useOrganization`,
                                    defaultMessage:
                                      'Switch to this organization'
                                  })}
                                </DropdownMenuItem>
                                <DropdownMenuSeparator />
                              </>
                            )}
                            <DropdownMenuItem
                              onClick={() => handleDialogOpen(organization.id!)}
                              data-testid={`organization-delete-${organization.id}`}
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
                      <span className="font-bold">{data?.items?.length}</span>
                    )
                  }
                )}
              </EntityDataTable.FooterText>
              <Pagination
                total={total}
                hasNextPage={
                  data?.items && data.items.length < pagination.limit
                }
                {...pagination}
              />
            </EntityDataTable.Footer>
          </EntityDataTable.Root>
        )}
      </Form>
    </div>
  )
}
