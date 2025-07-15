'use client'

import { SettingsDropdown } from '../settings-dropdown'
import { UserDropdown } from '../user-dropdown'
import { Separator } from '../ui/separator'
import Image from 'next/image'
import LerianLogo from '@/svg/lerian-logo.svg'
import { LedgerSelector } from '../ledger-selector'
import { useIntl } from 'react-intl'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '../ui/tooltip'
import { VersionStatus } from '@/core/application/dto/midaz-info-dto'
import { AlertTriangle, CheckCircle2 } from 'lucide-react'
import { useGetMidazInfo } from '@/client/midaz-info'

const VersionIcon = ({ status }: { status: VersionStatus }) => {
  const intl = useIntl()

  return (
    <TooltipProvider>
      <Tooltip delayDuration={500}>
        <TooltipTrigger>
          {status === VersionStatus.UpToDate && (
            <CheckCircle2 className="h-4 w-4 text-zinc-400" />
          )}
          {status === VersionStatus.Outdated && (
            <AlertTriangle className="h-4 w-4 text-yellow-500" />
          )}
        </TooltipTrigger>
        <TooltipContent>
          {status === VersionStatus.UpToDate &&
            intl.formatMessage({
              id: 'dialog.about.midaz.upToDate.tooltip',
              defaultMessage:
                'Your version is up to date and operating successfully.'
            })}
          {status === VersionStatus.Outdated &&
            intl.formatMessage({
              id: 'dialog.about.midaz.outdate.tooltip',
              defaultMessage:
                'A new version is available. We recommend updating.'
            })}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export const Header = () => {
  const intl = useIntl()
  const { data: midazInfo } = useGetMidazInfo()

  return (
    <div className="flex h-[60px] w-full items-center border-b bg-white py-5 pr-16">
      <nav className="flex w-full items-center justify-between gap-4 pl-16">
        <LedgerSelector />

        <div className="flex items-center gap-6">
          <div className="flex flex-row items-center gap-1">
            <p className="text-xs font-medium text-zinc-400">
              Midaz Console v.{process.env.NEXT_PUBLIC_MIDAZ_VERSION}
            </p>
            <VersionIcon status={midazInfo?.versionStatus!} />
          </div>

          <Separator orientation="vertical" className="h-10" />

          <p className="text-xs font-normal text-zinc-400">
            {intl.locale.toLocaleUpperCase()}
          </p>

          <SettingsDropdown />
          <UserDropdown />
        </div>
      </nav>
    </div>
  )
}

export const StaticHeader = () => {
  return (
    <div className="flex w-full items-center justify-center border-b bg-white py-6">
      <nav className="flex w-full max-w-[1090px] items-center gap-4">
        <Image
          src={LerianLogo}
          alt="Logo"
          height={40}
          width={40}
          className="rounded-lg"
        />

        <div className="flex text-base text-zinc-800">Midaz Console</div>
      </nav>
    </div>
  )
}
