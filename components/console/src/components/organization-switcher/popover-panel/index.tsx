import React from 'react'
import { cn } from '@/lib/utils'
import Link from 'next/link'

export type PopoverPanelProps = React.HTMLAttributes<HTMLDivElement>

const PopoverPanelTitle = (props: PopoverPanelProps) => {
  return (
    <div
      className="flex flex-col gap-2 text-base font-semibold text-zinc-700"
      {...props}
    />
  )
}

const PopoverPanelContent = (props: PopoverPanelProps) => {
  return <div className="flex flex-1 items-center justify-center" {...props} />
}

const PopoverPanelFooter = (props: PopoverPanelProps) => {
  return (
    <div
      className="mt-5 text-xs font-normal text-zinc-700 underline"
      {...props}
    />
  )
}

const PopoverPanel = (props: PopoverPanelProps) => {
  return (
    <div
      className="border-shadcn-200 flex h-full min-w-[160px] flex-1 flex-col gap-4 rounded-md border p-4"
      {...props}
    />
  )
}

const PopoverPanelActions = (props: PopoverPanelProps) => {
  return <div className="flex w-auto flex-col gap-4" {...props} />
}

export type PopoverPanelLinkProps = React.PropsWithChildren & {
  href: string
  icon?: React.ReactNode
  onClick: React.MouseEventHandler
  dense?: boolean
}

const PopoverPanelLink = ({
  href,
  icon,
  dense,
  onClick,
  children,
  ...others
}: PopoverPanelLinkProps) => {
  return (
    <Link
      href={href}
      onClick={onClick}
      className={cn(
        'hover:bg-shadcn-100 flex w-[320px] flex-1 items-center justify-between rounded-md bg-white p-4 text-black outline-hidden',
        dense && 'h-10 flex-auto'
      )}
      {...others}
    >
      <div
        className={cn(
          'flex items-center gap-2 text-sm font-medium text-zinc-600',
          dense && 'flex-row items-center'
        )}
      >
        {children}
      </div>

      {icon &&
        React.Children.map(React.Children.toArray(icon), (child) =>
          React.isValidElement(child)
            ? React.cloneElement(child as any, {
                className: 'text-shadcn-400',
                size: 24
              })
            : child
        )}
    </Link>
  )
}

export {
  PopoverPanel,
  PopoverPanelTitle,
  PopoverPanelContent,
  PopoverPanelFooter,
  PopoverPanelActions,
  PopoverPanelLink
}
