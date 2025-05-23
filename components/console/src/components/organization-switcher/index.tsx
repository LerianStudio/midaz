'use client'
import { useListOrganizations } from '@/client/organizations'
import { Popover } from '@/components/ui/popover'
import { useOrganization } from '@/context/organization-provider/organization-provider-client'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { useTheme } from '@/lib/theme'
import React, { useEffect } from 'react'
import { useIntl } from 'react-intl'
import { useSidebar } from '../sidebar/primitive'
import { Skeleton } from '../ui/skeleton'
import { OrganizationSwitcherContent } from './organization-switcher-content'
import { SwitcherTrigger } from './organization-switcher-trigger'
import MidazLogo from '/public/svg/brand-midaz.svg'

/**
 * TODO: Fix potential bug of user changing the organization and still having old stale data in the UI
 * @returns
 */

export const OrganizationSwitcher = () => {
  const intl = useIntl()
  const { isCollapsed } = useSidebar()
  const { data, isPending } = useListOrganizations({})
  const { currentOrganization, setOrganization } = useOrganization()
  const [open, setOpen] = React.useState(false)
  const [avatar, setAvatar] = React.useState<string>(MidazLogo)

  const handleChange = (organization: OrganizationEntity) => {
    setOrganization(organization)
    setOpen(false)
  }

  useEffect(() => {
    if (currentOrganization.metadata?.avatar) {
      return setAvatar(currentOrganization.metadata.avatar)
    }

    setAvatar(MidazLogo)
  }, [currentOrganization])

  if ((isPending && !data) || !currentOrganization) {
    return <Skeleton className="h-10 w-10" />
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <SwitcherTrigger
        open={open}
        name={currentOrganization.legalName}
        image={avatar}
        alt={intl.formatMessage({
          id: 'common.logoAlt',
          defaultMessage: 'Your organization logo'
        })}
        disabled={!data || data.items.length <= 1}
        collapsed={isCollapsed}
      />

      <OrganizationSwitcherContent
        currentOrganization={currentOrganization}
        status="active"
        alt={intl.formatMessage({
          id: 'common.logoAlt',
          defaultMessage: 'Your organization logo'
        })}
        image={avatar}
        data={data?.items || []}
        onChange={handleChange}
        onClose={() => setOpen(false)}
      />
    </Popover>
  )
}
