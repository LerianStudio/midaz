'use client'

import React, { useState } from 'react'
import { useRouter } from 'next/navigation'
import { useIntl } from 'react-intl'
import {
  Building,
  Globe,
  HelpCircle,
  Layers,
  Settings,
  Users
} from 'lucide-react'
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
              id: 'settings.title',
              defaultMessage: 'Settings'
            })}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => router.push('/settings')}>
            <DropdownMenuItemIcon>
              <Building />
            </DropdownMenuItemIcon>
            {intl.formatMessage({
              id: 'organizations.title',
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
                id: 'users.title',
                defaultMessage: 'Users'
              })}
            </DropdownMenuItem>
          </Enforce>
          <Enforce resource="applications" action="get">
            <DropdownMenuItem
              onClick={() => router.push('/settings?tab=applications')}
            >
              <DropdownMenuItemIcon>
                <Layers />
              </DropdownMenuItemIcon>
              {intl.formatMessage({
                id: 'applications.title',
                defaultMessage: 'Applications'
              })}
            </DropdownMenuItem>
          </Enforce>
          <DropdownMenuItem onClick={() => router.push('/settings?tab=system')}>
            <DropdownMenuItemIcon>
              <Globe />
            </DropdownMenuItemIcon>
            {intl.formatMessage({
              id: 'settings.tabs.system',
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
