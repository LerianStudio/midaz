'use client'

import Image from 'next/image'
import LerianLogo from '@/svg/lerian-logo.svg'

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
