import Image from 'next/image'
import MidazLogo from '/public/svg/brand-midaz.svg'
import { useIntl } from 'react-intl'
import { PopoverContent } from '../ui/popover'
import { StatusDisplay } from './status'
import { ArrowRight, Settings } from 'lucide-react'
import Link from 'next/link'
import {
  PopoverPanel,
  PopoverPanelActions,
  PopoverPanelContent,
  PopoverPanelFooter,
  PopoverPanelLink,
  PopoverPanelTitle
} from './popover-panel'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import React from 'react'

export type OrganizationSwitcherProps = {
  currentOrganization: OrganizationEntity
  data: OrganizationEntity[]
  status: 'active' | 'inactive'
  image: string
  alt: string
}

export type OrganizationSwitcherContentProps = OrganizationSwitcherProps & {
  onChange?: (organization: OrganizationEntity) => void
  onClose: () => void
}

export const OrganizationSwitcherContent = ({
  currentOrganization,
  status,
  alt,
  image,
  data,
  onChange,
  onClose
}: OrganizationSwitcherContentProps) => {
  const intl = useIntl()

  return (
    <PopoverContent className="flex w-auto gap-4" side="right">
      <PopoverPanel>
        <PopoverPanelTitle>
          {currentOrganization.legalName}
          <StatusDisplay status={status} />
        </PopoverPanelTitle>
        <PopoverPanelContent>
          <Image
            src={image}
            alt={alt}
            className="rounded-full"
            height={112}
            width={112}
          />
        </PopoverPanelContent>
        <PopoverPanelFooter>
          <Link
            href={`/settings/organizations/${currentOrganization.id}`}
            onClick={onClose}
          >
            {intl.formatMessage({
              id: 'common.edit',
              defaultMessage: 'Edit'
            })}
          </Link>
        </PopoverPanelFooter>
      </PopoverPanel>

      {data?.length > 1 && (
        <PopoverPanelActions>
          {data.map((organization) => (
            <PopoverPanelLink
              key={organization.id}
              href="#"
              icon={<ArrowRight />}
              dense={data.length >= 4}
              onClick={() => onChange?.(organization)}
            >
              <Image
                src={
                  organization.metadata?.avatar
                    ? organization.metadata?.avatar
                    : MidazLogo
                }
                alt=""
                width={28}
                className="rounded-full"
                height={28}
              />

              {organization.legalName}
            </PopoverPanelLink>
          ))}

          <PopoverPanelLink
            href="/settings?tab=organizations"
            dense={data.length >= 4}
            onClick={onClose}
            icon={<Settings />}
          >
            {intl.formatMessage({
              id: 'entity.organization',
              defaultMessage: 'Organization'
            })}
          </PopoverPanelLink>
        </PopoverPanelActions>
      )}
    </PopoverContent>
  )
}
