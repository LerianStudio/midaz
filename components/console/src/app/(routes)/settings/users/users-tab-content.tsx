'use client'

import { Button } from '@/components/ui/button'
import { MoreVertical } from 'lucide-react'
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
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { EntityDataTable } from '@/components/entity-data-table'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { UsersSheet } from './users-sheet'
import { useDeleteUser, useListUsers } from '@/client/users'
import { Skeleton } from '@/components/ui/skeleton'
import { UserResponseDto } from '@/core/application/dto/user-dto'
import { useSession } from 'next-auth/react'
import { useToast } from '@/hooks/use-toast'
import { UsersType } from '@/types/users-type'

export const UsersTabContent = () => {
  const intl = useIntl()
  const { data: session } = useSession()
  const { data: users, refetch, isLoading } = useListUsers({})
  const { toast } = useToast()
  const { handleCreate, handleEdit, sheetProps } =
    useCreateUpdateSheet<UsersType>({
      enableRouting: true
    })

  const { mutate: deleteUser, isPending: deletePending } = useDeleteUser({
    onSuccess: () => {
      handleDialogClose()
      refetch()
      toast({
        description: intl.formatMessage({
          id: 'success.users.delete',
          defaultMessage: 'User successfully deleted'
        }),
        variant: 'success'
      })
    }
  })

  const { handleDialogOpen, dialogProps, handleDialogClose } = useConfirmDialog(
    {
      onConfirm: (id: string) => deleteUser({ id })
    }
  )

  return (
    <div>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'common.confirmDeletion',
          defaultMessage: 'Confirm Deletion'
        })}
        description={intl.formatMessage({
          id: 'users.delete.description',
          defaultMessage:
            'You are about to permanently delete this user. This action cannot be undone. Do you wish to continue?'
        })}
        loading={deletePending}
        {...dialogProps}
      />

      <EntityBox.Root>
        <EntityBox.Header
          title={intl.formatMessage({
            id: 'users.title',
            defaultMessage: 'Users'
          })}
          subtitle={intl.formatMessage({
            id: 'users.subtitle',
            defaultMessage: 'Manage the users of Midaz.'
          })}
          tooltip={intl.formatMessage({
            id: 'users.tooltip',
            defaultMessage:
              'Users who will be able to interact with the Organizations and Ledgers you create.'
          })}
        />

        <EntityBox.Actions>
          <Button onClick={handleCreate}>
            {intl.formatMessage({
              id: 'users.listingTemplate.addButton',
              defaultMessage: 'New User'
            })}
          </Button>
        </EntityBox.Actions>
      </EntityBox.Root>

      {isLoading && <Skeleton className="mt-4 h-[390px] w-full bg-zinc-200" />}

      {!isLoading && users?.length > 0 && (
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
                      id: `common.email`,
                      defaultMessage: 'E-mail'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: `entity.user.groups`,
                      defaultMessage: 'Group'
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
                {users?.map((user: UserResponseDto) => (
                  <TableRow key={user.id}>
                    <TableCell>
                      {!user.firstName || !user.lastName ? (
                        '-'
                      ) : (
                        <>
                          {user.firstName} {user.lastName}
                        </>
                      )}
                    </TableCell>
                    <TableCell>{user.email}</TableCell>
                    <TableCell>{user.groups[0]}</TableCell>
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
                          <DropdownMenuItem onClick={() => handleEdit(user)}>
                            {intl.formatMessage({
                              id: `common.details`,
                              defaultMessage: 'Details'
                            })}
                          </DropdownMenuItem>
                          {user.id !== session?.user?.id && (
                            <React.Fragment>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem
                                onClick={() => handleDialogOpen(user.id!)}
                              >
                                {intl.formatMessage({
                                  id: `common.delete`,
                                  defaultMessage: 'Delete'
                                })}
                              </DropdownMenuItem>
                            </React.Fragment>
                          )}
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
                  id: 'users.showing',
                  defaultMessage:
                    'Showing {count} {number, plural, =0 {users} one {user} other {users}}.'
                },
                {
                  number: users?.length,
                  count: <span className="font-bold">{users?.length}</span>
                }
              )}
            </EntityDataTable.FooterText>
          </EntityDataTable.Footer>
        </EntityDataTable.Root>
      )}

      <UsersSheet {...sheetProps} onSuccess={refetch} />
    </div>
  )
}
