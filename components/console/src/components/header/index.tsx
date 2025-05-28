'use client'

import { SettingsDropdown } from '../settings-dropdown'
import { UserDropdown } from '../user-dropdown'
import { Separator } from '../ui/separator'
import Image from 'next/image'
import LerianLogo from '@/svg/lerian-logo.svg'
import { LedgerSelector } from '../ledger-selector'
import { useIntl } from 'react-intl'

export const Header = () => {
  const intl = useIntl()

  return (
    <div className="flex h-[60px] w-full items-center border-b bg-white py-5 pr-16">
      <nav className="flex w-full items-center justify-between gap-4 pl-16">
        <LedgerSelector />

        <div className="flex items-center gap-6">
          <p className="text-xs font-medium text-zinc-400">
            Midaz Console{' '}
            <span className="text-xs font-normal text-zinc-400">
              v.{process.env.VERSION}
            </span>
          </p>

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
