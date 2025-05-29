import React from 'react'
import { cn } from '@/lib/utils'

export type SidebarHeaderProps = React.HTMLAttributes<HTMLDivElement> & {
  className?: string
  collapsed?: boolean
}

const SidebarHeader = React.forwardRef<HTMLDivElement, SidebarHeaderProps>(
  ({ className, collapsed, ...props }: SidebarHeaderProps, ref) => (
    <div
      ref={ref}
      data-collapsed={collapsed}
      className={cn(
        'flex h-[60px] items-center border-b bg-white px-4 dark:bg-cod-gray-950',
        collapsed && 'justify-center p-0'
      )}
      {...props}
    />
  )
)
SidebarHeader.displayName = 'SidebarHeader'

export type SidebarContentProps = React.HTMLAttributes<HTMLDivElement> & {
  className?: string
}

const SidebarContent = React.forwardRef<HTMLDivElement, SidebarContentProps>(
  ({ className, ...props }: SidebarContentProps, ref) => (
    <div
      ref={ref}
      className={cn(
        'group flex flex-1 flex-col gap-4 bg-white px-4 pt-4',
        'group-data-[collapsed=true]/sidebar:items-center group-data-[collapsed=true]/sidebar:px-2',
        'group-data-[collapsed=false]/sidebar:min-w-[244px]',
        className
      )}
      {...props}
    />
  )
)
SidebarContent.displayName = 'SidebarContent'

export type SidebarGroupProps = {
  className?: string
} & React.HTMLAttributes<HTMLElement>

const SidebarGroup = React.forwardRef<HTMLElement, SidebarGroupProps>(
  ({ className, ...props }: SidebarGroupProps, ref) => (
    <nav
      ref={ref}
      className={cn(
        'grid gap-1',
        'group-data[collapsed=true]/sidebar:justify-center',
        className
      )}
      {...props}
    />
  )
)
SidebarGroup.displayName = 'SidebarGroup'

export type SidebarGroupTitleProps = React.PropsWithChildren & {
  collapsed?: boolean
}

const SidebarGroupTitle = ({ collapsed, children }: SidebarGroupTitleProps) => {
  if (collapsed) {
    return null
  }

  return (
    <div className="my-2 px-2">
      <p className="text-xs font-semibold uppercase tracking-[1.1px] text-zinc-500">
        {children}
      </p>
    </div>
  )
}

export type SidebarFooterProps = {
  className?: string
} & React.HTMLAttributes<HTMLElement>

const SidebarFooter = React.forwardRef<HTMLElement, SidebarFooterProps>(
  ({ className, ...props }: SidebarFooterProps, ref) => (
    <nav
      ref={ref}
      className={cn(
        'flex w-full justify-center border-t border-shadcn-200 bg-white p-4',
        className
      )}
      {...props}
    />
  )
)
SidebarFooter.displayName = 'SidebarFooter'

export {
  SidebarHeader,
  SidebarContent,
  SidebarGroup,
  SidebarGroupTitle,
  SidebarFooter
}
