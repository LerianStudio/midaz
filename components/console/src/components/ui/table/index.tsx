import * as React from 'react'

import { cn } from '@/lib/utils'

function TableContainer({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="data-table-container"
      className={cn('mt-4', className)}
      {...props}
    />
  )
}

function Table({ className, ...props }: React.ComponentProps<'table'>) {
  return (
    <div className="relative w-full overflow-auto">
      <table
        data-slot="data-table"
        className={cn('w-full caption-bottom text-sm', className)}
        {...props}
      />
    </div>
  )
}

function TableHeader({ className, ...props }: React.ComponentProps<'thead'>) {
  return (
    <thead
      data-slot="data-table-header"
      className={cn('[&_tr]:border-b hover:[&_tr]:bg-transparent', className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: React.ComponentProps<'tbody'>) {
  return (
    <tbody
      data-slot="data-table-body"
      className={cn('', className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: React.ComponentProps<'tfoot'>) {
  return (
    <tfoot
      data-slot="data-table-footer"
      className={cn(
        'bg-muted/50 border-t font-medium last:[&>tr]:border-b-0',
        className
      )}
      {...props}
    />
  )
}

export type TableRowProps = React.ComponentProps<'tr'> & {
  active?: boolean
  button?: boolean
}

function TableRow({ className, active, button, ...props }: TableRowProps) {
  return (
    <tr
      data-slot="data-table-row"
      className={cn(
        'data-[state=selected]:bg-muted border-b transition-colors hover:bg-[#FAFAFA]',
        {
          'cursor-pointer': button,
          'bg-[#FEED0280] hover:bg-[#FEED0280]': active
        },
        className
      )}
      {...props}
    />
  )
}

export type TableHeadProps = React.ComponentProps<'th'> & {
  align?: 'left' | 'center' | 'right'
}

function TableHead({ className, align, ...props }: TableHeadProps) {
  return (
    <th
      data-slot="data-table-head"
      className={cn(
        'h-12 px-6 py-4 text-left align-middle font-medium text-[#52525B] [&:has([role=checkbox])]:pr-0',
        {
          'text-center': align === 'center',
          'text-right': align === 'right'
        },
        className
      )}
      {...props}
    />
  )
}

function TableCell({
  className,
  onClick,
  ...props
}: React.ComponentProps<'td'>) {
  return (
    <td
      data-slot="data-table-cell"
      className={cn(
        'group/table-cell text-shadcn-600 px-6 py-4 align-middle text-sm font-normal [&:has([role=checkbox])]:pr-0',
        {
          'hover:underline': onClick
        },
        className
      )}
      onClick={onClick}
      {...props}
    />
  )
}

function TableCellWrapper({
  className,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="data-table-cell-wrapper"
      className={cn('flex items-center', className)}
      {...props}
    />
  )
}

function TableCellAction({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="data-table-cell-action"
      className={cn(
        'ml-4 w-fit opacity-0 transition-opacity group-hover/table-cell:opacity-100',
        className
      )}
      {...props}
    />
  )
}

function TableCaption({
  className,
  ...props
}: React.ComponentProps<'caption'>) {
  return (
    <caption
      data-slot="data-table-caption"
      className={cn('text-muted-foreground mt-4 text-sm', className)}
      {...props}
    />
  )
}

export {
  TableContainer,
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCellWrapper,
  TableCellAction,
  TableCaption
}
