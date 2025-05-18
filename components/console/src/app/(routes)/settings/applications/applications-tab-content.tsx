'use client'

import { Button } from '@/components/ui/button'
import { MoreVertical, AlertTriangle } from 'lucide-react'
import React, { useState } from 'react'
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
import { useToast } from '@/hooks/use-toast'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { ApplicationsSheet, ApplicationType } from './applications-sheet'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

const mockApplications: ApplicationType[] = [
  {
    id: '1',
    name: 'Web Dashboard',
    description: 'Main web dashboard application for Midaz UI',
    clientId: 'web-dashboard-123',
    clientSecret: 'secret-key-abc-123',
    status: 'active',
    createdAt: new Date('2023-01-15T10:30:00')
  },
  {
    id: '2',
    name: 'Mobile App',
    description: 'Native mobile application for iOS and Android',
    clientId: 'mobile-app-456',
    clientSecret: 'secret-key-def-456',
    status: 'active',
    createdAt: new Date('2023-03-22T14:45:00')
  },
  {
    id: '3',
    name: 'Legacy Integration',
    description: 'Integration with legacy accounting systems',
    clientId: 'legacy-int-789',
    clientSecret: 'secret-key-ghi-789',
    status: 'inactive',
    createdAt: new Date('2022-11-05T09:15:00')
  }
]

export const ApplicationsTabContent = () => {
  const intl = useIntl()
  const { toast } = useToast()
  const [isLoading, setIsLoading] = useState(false)
  const [applications, setApplications] = useState(mockApplications)

  const { handleCreate, handleEdit, sheetProps } =
    useCreateUpdateSheet<ApplicationType>({
      enableRouting: true
    })

  const deleteApplication = (id: string) => {
    setApplications(applications.filter((app) => app.id !== id))
    toast({
      description: intl.formatMessage({
        id: 'success.applications.delete',
        defaultMessage: 'Application successfully deleted'
      }),
      variant: 'success'
    })
  }

  const handleSheetSuccess = () => {
    console.log('Application operation completed successfully')
  }

  const { handleDialogOpen, dialogProps, handleDialogClose } = useConfirmDialog(
    {
      onConfirm: (id: string) => {
        deleteApplication(id)
        handleDialogClose()
      }
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
        loading={false}
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

      <Alert variant="warning" className="mb-6">
        <AlertTriangle size={24} />
        <AlertTitle className="ml-2 text-sm font-bold text-yellow-800">
          Security Warning
        </AlertTitle>
        <AlertDescription className="text-sm text-yellow-800 opacity-70">
          <ul className="ml-5 mt-2 list-disc space-y-1">
            <li className="font-bold">
              {intl.formatMessage({
                id: 'applications.security.doNotShare',
                defaultMessage:
                  'Do not share your clientId or clientSecret publicly. These credentials grant access to your application and must be kept confidential.'
              })}
            </li>
            <li>
              {intl.formatMessage({
                id: 'applications.security.secureStorage',
                defaultMessage: 'Store these keys in a secure location.'
              })}
            </li>
            <li>
              {intl.formatMessage(
                {
                  id: 'applications.security.doNotDelete',
                  defaultMessage:
                    "{doNotDelete} the application unless you're sure. Deleting it revokes access to all connected services."
                },
                {
                  doNotDelete: (
                    <span className="font-bold">
                      {intl.formatMessage({
                        id: 'applications.security.doNotDelete',
                        defaultMessage: 'Do not delete'
                      })}
                    </span>
                  )
                }
              )}
            </li>
            <li>
              {intl.formatMessage({
                id: 'applications.security.rotateCredentials',
                defaultMessage:
                  'Rotate your credentials if you suspect they were compromised.'
              })}
            </li>
            <li>
              {intl.formatMessage(
                {
                  id: 'applications.security.neverExpose',
                  defaultMessage:
                    '{neverExpose} these keys in frontend code or public repositories.'
                },
                {
                  neverExpose: (
                    <span className="font-bold">
                      {intl.formatMessage({
                        id: 'applications.security.neverExpose',
                        defaultMessage: 'Never expose'
                      })}
                    </span>
                  )
                }
              )}
            </li>
          </ul>
        </AlertDescription>
      </Alert>

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
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.name',
                      defaultMessage: 'Name'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'applications.clientId',
                      defaultMessage: 'Client ID'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'applications.clientSecret',
                      defaultMessage: 'Client Secret'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.creationDate',
                      defaultMessage: 'Creation Date'
                    })}
                  </TableHead>
                  <TableHead className="w-0">
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
                    <TableCell>{application.clientId}</TableCell>
                    <TableCell>
                      <span className="font-mono text-xs">
                        {application.clientSecret ? '••••••••••••' : '—'}
                      </span>
                    </TableCell>
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
                            <MoreVertical size={16} onClick={() => {}} />
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

      <ApplicationsSheet {...sheetProps} onSuccess={handleSheetSuccess} />
    </div>
  )
}
