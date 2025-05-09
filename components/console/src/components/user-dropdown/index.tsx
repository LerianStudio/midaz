'use client'

import React, { useState } from 'react'
import SettingsDialog from '../settings-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuItemIcon,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '../ui/dropdown-menu'
import { useIntl } from 'react-intl'
import { Book, CircleUser, LogOut } from 'lucide-react'
import { signOut, useSession } from 'next-auth/react'
import { useCreateUpdateSheet } from '../sheet/use-create-update-sheet'
import { useUserById } from '@/client/users'

export const UserDropdown = () => {
  const intl = useIntl()
  const { data: session } = useSession()
  const { handleEdit, sheetProps } = useCreateUpdateSheet<any>({
    enableRouting: true
  })
  const [openSettings, setOpenSettings] = useState(false)

  const isAuthPluginEnabled = process.env.PLUGIN_AUTH_ENABLED === 'true'

  const userData = isAuthPluginEnabled
    ? useUserById({ userId: session?.user?.id })
    : null

  const handleOpenUserSheet = () => {
    handleEdit(userData?.data)
  }

  return (
    <div>
      <DropdownMenu>
        <DropdownMenuTrigger>
          <CircleUser className="h-8 w-8 text-shadcn-400" />
        </DropdownMenuTrigger>
        <DropdownMenuContent className="min-w-[241px]">
          <DropdownMenuLabel>
            {session?.user?.name ||
              intl.formatMessage({
                id: 'common.user',
                defaultMessage: 'User'
              })}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild>
            <a
              href="https://docs.lerian.studio/"
              target="_blank"
              rel="noopener noreferrer"
            >
              <DropdownMenuItemIcon>
                <Book />
              </DropdownMenuItemIcon>
              {intl.formatMessage({
                id: 'header.userDropdown.documentation',
                defaultMessage: 'Documentation Hub'
              })}
            </a>
          </DropdownMenuItem>
          <DropdownMenuSeparator />

          {isAuthPluginEnabled && (
            <DropdownMenuItem
              onClick={() => signOut({ callbackUrl: '/signin' })}
            >
              <DropdownMenuItemIcon>
                <LogOut />
              </DropdownMenuItemIcon>
              {intl.formatMessage({
                id: 'header.userDropdown.logout',
                defaultMessage: 'Logout'
              })}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <SettingsDialog open={openSettings} setOpen={setOpenSettings} />
    </div>
  )
}
