'use client'

import { Button } from '@/components/ui/button'
import { MoreVertical, AlertTriangle } from 'lucide-react'
import React from 'react'
import { useIntl } from 'react-intl'
import { EntityBox } from '@/components/entity-box'
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
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { EntityDataTable } from '@/components/entity-data-table'
import { Skeleton } from '@/components/ui/skeleton'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { ApplicationsSheet } from './applications-sheet'
import { useApplications, useDeleteApplication } from '@/client/applications'
import { ApplicationResponseDto } from '@/core/application/dto/application-dto'
import { useToast } from '@/hooks/use-toast'
import { ApplicationsSecurityAlert } from './applications-security-alert'
import { CopyableTableCell } from '@/components/table/copyable-table-cell'

export const ApplicationsTabContent = () => {
  const intl = useIntl()
  const { data: applications = [], isLoading, refetch } = useApplications()
  const { toast } = useToast()

  const { handleCreate, handleEdit, sheetProps } =
    useCreateUpdateSheet<ApplicationResponseDto>({
      enableRouting: true
    })

  const { mutate: deleteApplication, isPending: deleteApplicationPending } =
    useDeleteApplication({
      onSuccess: () => {
        handleDialogClose()
        refetch()
        toast({
          description: intl.formatMessage({
            id: 'success.applications.delete',
            defaultMessage: 'Application successfully deleted'
          }),
          variant: 'success'
        })
      }
    })

  const { handleDialogOpen, dialogProps, handleDialogClose } = useConfirmDialog(
    {
      onConfirm: (id: string) => deleteApplication({ id })
    }
  )

  return (
    <div>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'dialog.delete.confirmTitle',
          defaultMessage: 'Are you sure you want to delete?'
        })}
        description={intl.formatMessage({
          id: 'applications.delete.confirmDescription',
          defaultMessage:
            'Deletion revokes access to all connected services. It is irreversible and permanently removes the Application.'
        })}
        icon={<AlertTriangle size={24} className="text-yellow-500" />}
        loading={deleteApplicationPending}
        cancelLabel={intl.formatMessage({
          id: 'common.changeMyMind',
          defaultMessage: 'I changed my mind'
        })}
        confirmLabel={intl.formatMessage({
          id: 'dialog.delete.confirmLabel',
          defaultMessage: 'Yes, delete it'
        })}
        {...dialogProps}
      />

      <ApplicationsSecurityAlert />

      <EntityBox.Root>
        <EntityBox.Header
          title={intl.formatMessage({
            id: 'applications.title',
            defaultMessage: 'Applications'
          })}
          subtitle={intl.formatMessage({
            id: 'applications.subtitle',
            defaultMessage: 'Manage the applications in Midaz.'
          })}
          tooltip={intl.formatMessage({
            id: 'applications.tooltip',
            defaultMessage:
              'It is an entity that represents an OAuth client — in other words, an external system or service that connects to your API through authentication via an Identity Provider. Each Application has its own credentials (client ID and secret) used to authorize and authenticate access to the APIs.'
          })}
          tooltipWidth="655px"
        />

        <EntityBox.Actions>
          <Button onClick={handleCreate}>
            {intl.formatMessage({
              id: 'applications.listingTemplate.addButton',
              defaultMessage: 'New Application'
            })}
          </Button>
        </EntityBox.Actions>
      </EntityBox.Root>

      {isLoading && <Skeleton className="mt-4 h-[390px] w-full bg-zinc-200" />}

      {!isLoading && applications.length > 0 && (
        <EntityDataTable.Root>
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[22%]">
                    {intl.formatMessage({
                      id: 'common.name',
                      defaultMessage: 'Name'
                    })}
                  </TableHead>
                  <TableHead className="w-[21%]">
                    {intl.formatMessage({
                      id: 'applications.clientId',
                      defaultMessage: 'ClientId'
                    })}
                  </TableHead>
                  <TableHead className="w-[33%]">
                    {intl.formatMessage({
                      id: 'applications.clientSecret',
                      defaultMessage: 'ClientSecret'
                    })}
                  </TableHead>
                  <TableHead className="w-[21%]">
                    {intl.formatMessage({
                      id: 'common.creationDate',
                      defaultMessage: 'Creation Date'
                    })}
                  </TableHead>
                  <TableHead className="w-[3%]">
                    {intl.formatMessage({
                      id: 'common.actions',
                      defaultMessage: 'Actions'
                    })}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {applications.map((application) => (
                  <TableRow key={application.id}>
                    <TableCell>{application.name}</TableCell>
                    <CopyableTableCell value={application.clientId} />
                    <CopyableTableCell value={application.clientSecret} />
                    <TableCell>
                      {application.createdAt
                        ? intl.formatDate(application.createdAt, {
                            day: '2-digit',
                            month: '2-digit',
                            year: 'numeric'
                          })
                        : '—'}
                    </TableCell>
                    <TableCell align="center">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="secondary"
                            className="h-auto w-max p-2"
                          >
                            <MoreVertical size={16} />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            onClick={() => handleEdit(application)}
                          >
                            {intl.formatMessage({
                              id: `common.details`,
                              defaultMessage: 'Details'
                            })}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => handleDialogOpen(application.id)}
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
                  id: 'applications.showing',
                  defaultMessage:
                    'Showing {count} {number, plural, =0 {applications} one {application} other {applications}}.'
                },
                {
                  number: applications.length,
                  count: (
                    <span className="font-bold">{applications.length}</span>
                  )
                }
              )}
            </EntityDataTable.FooterText>
          </EntityDataTable.Footer>
        </EntityDataTable.Root>
      )}

      <ApplicationsSheet {...sheetProps} onSuccess={refetch} />
    </div>
  )
}
