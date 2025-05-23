'use client'

import { usePathname } from 'next/navigation'
import React from 'react'
import { SidebarItemButton } from './sidebar-item-button'
import { SidebarItemIconButton } from './sidebar-item-icon-button'
import { useSidebar } from './sidebar-provider'
import { useIntl } from 'react-intl'

type SidebarItemProps = {
  title: string
  icon: React.ReactNode
  href: string
  disabled?: boolean
  disabledReason?: string
}

export const SidebarItem = ({
  disabled,
  href,
  disabledReason,
  ...others
}: SidebarItemProps) => {
  const pathName = usePathname()
  const { isCollapsed } = useSidebar()
  const intl = useIntl()

  const defaultDisabledReason =
    disabledReason ||
    intl.formatMessage({
      id: 'sidebar.disabled.reason',
      defaultMessage: 'No ledger selected. To access, create a ledger.'
    })

  const isActive = (href: string) => pathName === href

  if (isCollapsed) {
    return (
      <SidebarItemIconButton
        href={href}
        active={isActive(href)}
        disabled={disabled}
        disabledReason={defaultDisabledReason}
        {...others}
      />
    )
  }

  return (
    <SidebarItemButton
      href={href}
      active={isActive(href)}
      disabled={disabled}
      disabledReason={defaultDisabledReason}
      {...others}
    />
  )
}
