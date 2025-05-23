'use client'

import { Button } from '@/components/ui/button'
import { MoreVertical, Plus } from 'lucide-react'
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
import { Badge } from '@/components/ui/badge'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { EntityDataTable } from '@/components/entity-data-table'
import { Skeleton } from '@/components/ui/skeleton'

export const OrganizationsTabContent = () => {
  const intl = useIntl()
  const { data, refetch, isLoading } = useListOrganizations({})
  const router = useRouter()

  const { mutate: deleteOrganization, isPending: deletePending } =
    useDeleteOrganization({
      onSuccess: () => {
        handleDialogClose()
        refetch()
      }
    })

  const { handleDialogOpen, dialogProps, handleDialogClose } = useConfirmDialog(
    {
      onConfirm: (id: string) => deleteOrganization({ id })
    }
  )

  const handleEdit = (organization: OrganizationEntity) => {
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
          defaultMessage:
            'You are about to permanently delete this organization. This action cannot be undone. Do you wish to continue?'
        })}
        loading={deletePending}
        {...dialogProps}
      />

      <EntityBox.Root>
        <EntityBox.Header
          title={intl.formatMessage({
            id: 'organizations.title',
            defaultMessage: 'Settings'
          })}
          subtitle={intl.formatMessage({
            id: 'organizations.subtitle',
            defaultMessage: 'View and manage Organizations.'
          })}
        />

        <EntityBox.Actions>
          <Button onClick={() => handleCreateOrganization()}>
            {intl.formatMessage({
              id: 'organizations.listingTemplate.addButton',
              defaultMessage: 'New Organization'
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

      {isLoading && <Skeleton className="mt-4 h-[390px] w-full bg-zinc-200" />}

      {!isLoading && data?.items && data.items.length > 0 && (
        <EntityDataTable.Root>
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.id',
                      defaultMessage: 'ID'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: `entity.organization.legalName`,
                      defaultMessage: 'Legal Name'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: `entity.organization.doingBusinessAs`,
                      defaultMessage: 'Trade Name'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: `entity.organization.legalDocument`,
                      defaultMessage: 'Document'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: `entity.organization.status`,
                      defaultMessage: 'Status'
                    })}
                  </TableHead>
                  <TableHead>
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
                    <TableCell>{organization.id}</TableCell>
                    <TableCell>{organization.legalName}</TableCell>
                    <TableCell>{organization.doingBusinessAs}</TableCell>
                    <TableCell>{organization.legalDocument}</TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          organization.status.code === 'ACTIVE'
                            ? 'active'
                            : 'inactive'
                        }
                      >
                        {organization.status.code === 'ACTIVE'
                          ? intl.formatMessage({
                              id: 'common.active',
                              defaultMessage: 'Active'
                            })
                          : intl.formatMessage({
                              id: 'common.inactive',
                              defaultMessage: 'Inactive'
                            })}
                      </Badge>
                    </TableCell>
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
                              id: `common.edit`,
                              defaultMessage: 'Edit'
                            })}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            onClick={() => handleDialogOpen(organization.id!)}
                          >
                            {intl.formatMessage({
                              id: `common.delete`,
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

          <EntityDataTable.Footer>
            <EntityDataTable.FooterText>
              {intl.formatMessage(
                {
                  id: 'organizations.showing',
                  defaultMessage:
                    'Showing {count} {number, plural, =0 {organizations} one {organization} other {organizations}}.'
                },
                {
                  number: data?.items?.length,
                  count: (
                    <span className="font-bold">{data?.items?.length}</span>
                  )
                }
              )}
            </EntityDataTable.FooterText>
          </EntityDataTable.Footer>
        </EntityDataTable.Root>
      )}
    </div>
  )
}
