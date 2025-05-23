'use client'

import React, { useState } from 'react'
import { useRouter } from 'next/navigation'
import { useIntl } from 'react-intl'
import { Building, Globe, HelpCircle, Settings, Users } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuItemIcon,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '../ui/dropdown-menu'
import { AboutMidazDialog } from './about-midaz-dialog'
import { Enforce } from '@/providers/permission-provider/enforce'

export const SettingsDropdown = () => {
  const intl = useIntl()
  const router = useRouter()
  const [aboutOpen, setAboutOpen] = useState(false)

  return (
    <React.Fragment>
      <DropdownMenu>
        <DropdownMenuTrigger>
          <Settings className="text-shadcn-400" size={24} />
        </DropdownMenuTrigger>
        <DropdownMenuContent className="min-w-[241px]">
          <DropdownMenuLabel>
            {intl.formatMessage({
              id: 'settingsDropdown.settings',
              defaultMessage: 'Settings'
            })}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => router.push('/settings')}>
            <DropdownMenuItemIcon>
              <Building />
            </DropdownMenuItemIcon>
            {intl.formatMessage({
              id: 'settingsDropdown.organizations',
              defaultMessage: 'Organizations'
            })}
          </DropdownMenuItem>
          <Enforce resource="users" action="get">
            <DropdownMenuItem
              onClick={() => router.push('/settings?tab=users')}
            >
              <DropdownMenuItemIcon>
                <Users />
              </DropdownMenuItemIcon>
              {intl.formatMessage({
                id: 'settingsDropdown.users',
                defaultMessage: 'Users'
              })}
            </DropdownMenuItem>
          </Enforce>
          <DropdownMenuItem onClick={() => router.push('/settings?tab=system')}>
            <DropdownMenuItemIcon>
              <Globe />
            </DropdownMenuItemIcon>
            {intl.formatMessage({
              id: 'settingsDropdown.system',
              defaultMessage: 'System'
            })}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => setAboutOpen(true)}>
            <DropdownMenuItemIcon>
              <HelpCircle />
            </DropdownMenuItemIcon>
            {intl.formatMessage({
              id: 'settingsDropdown.about.midaz',
              defaultMessage: 'About Midaz'
            })}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <AboutMidazDialog open={aboutOpen} setOpen={setAboutOpen} />
    </React.Fragment>
  )
}
