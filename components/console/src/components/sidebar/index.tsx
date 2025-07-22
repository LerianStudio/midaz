'use client'

import React from 'react'
import {
  ArrowLeftRight,
  Briefcase,
  Coins,
  DollarSign,
  Group,
  Home,
  LibraryBig
} from 'lucide-react'

import * as LucideIcons from 'lucide-react'
import { OrganizationSwitcher } from '../organization-switcher'
import { useIntl } from 'react-intl'
import {
  SidebarItem,
  SidebarContent,
  SidebarGroup,
  SidebarGroupTitle,
  SidebarHeader,
  useSidebar,
  SidebarExpandButton,
  SidebarRoot
} from './primitive'
import { Separator } from '../ui/separator'
import { useOrganization } from '@lerianstudio/console-layout'
import { useGetPluginMenus } from '@/client/plugin-menu'
import { PluginManifestDto } from '@/core/application/dto/plugin-manifest-dto'

export const Sidebar = () => {
  const intl = useIntl()
  const { isCollapsed } = useSidebar()
  const [isMobileWidth, setIsMobileWidth] = React.useState(false)
  const { currentLedger } = useOrganization()
  const { data } = useGetPluginMenus()

  React.useEffect(() => {
    const handleResize = () => {
      setIsMobileWidth(window.innerWidth < 768)
    }

    handleResize()
    window.addEventListener('resize', handleResize)

    return () => window.removeEventListener('resize', handleResize)
  }, [])

  return (
    <SidebarRoot>
      <SidebarHeader>
        <OrganizationSwitcher />
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarItem
            title={intl.formatMessage({
              id: 'sideBar.home',
              defaultMessage: 'Home'
            })}
            icon={<Home />}
            href="/"
          />

          <SidebarItem
            title={intl.formatMessage({
              id: 'sideBar.ledgers',
              defaultMessage: 'Ledgers'
            })}
            icon={<LibraryBig />}
            href="/ledgers"
          />
        </SidebarGroup>

        {isCollapsed && <Separator />}

        <SidebarGroup>
          <SidebarGroupTitle collapsed={isCollapsed}>
            {intl.formatMessage({
              id: 'sideBar.ledger.title',
              defaultMessage: 'Ledger'
            })}
          </SidebarGroupTitle>

          <SidebarItem
            title={intl.formatMessage({
              id: 'common.assets',
              defaultMessage: 'Assets'
            })}
            icon={<DollarSign />}
            href="/assets"
            disabled={Object.keys(currentLedger).length === 0}
          />

          <SidebarItem
            title={intl.formatMessage({
              id: 'sideBar.ledger.accounts',
              defaultMessage: 'Accounts'
            })}
            icon={<Coins />}
            href="/accounts"
            disabled={Object.keys(currentLedger).length === 0}
          />

          <SidebarItem
            title={intl.formatMessage({
              id: 'common.segments',
              defaultMessage: 'Segments'
            })}
            icon={<Group />}
            href="/segments"
            disabled={Object.keys(currentLedger).length === 0}
          />

          <SidebarItem
            title={intl.formatMessage({
              id: 'sideBar.accountHolders.portfolios',
              defaultMessage: 'Portfolios'
            })}
            icon={<Briefcase />}
            href="/portfolios"
            disabled={Object.keys(currentLedger).length === 0}
          />

          <SidebarItem
            title={intl.formatMessage({
              id: 'common.transactions',
              defaultMessage: 'Transactions'
            })}
            icon={<ArrowLeftRight />}
            href="/transactions"
            disabled={Object.keys(currentLedger).length === 0}
          />
        </SidebarGroup>

        {data && data.length > 0 && (
          <SidebarGroup>
            <SidebarGroupTitle collapsed={isCollapsed}>
              Plugins
            </SidebarGroupTitle>
            {data.map((plugin: PluginManifestDto) => {
              if (!plugin.enabled) {
                return
              }

              const Icon =
                (LucideIcons as unknown as Record<string, React.ElementType>)[
                  plugin.icon
                ] || LucideIcons.Landmark
              return (
                <SidebarItem
                  key={plugin.name}
                  title={plugin.title}
                  icon={<Icon />}
                  href={`${plugin.route}`}
                />
              )
            })}
          </SidebarGroup>
        )}
      </SidebarContent>

      {!isMobileWidth && <SidebarExpandButton />}
    </SidebarRoot>
  )
}
