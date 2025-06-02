import * as React from 'react'

import { cn } from '@/lib/utils'

const TableContainer = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div className={cn('mt-4', className)} {...props} />
))
TableContainer.displayName = 'TableContainer'

const Table = React.forwardRef<
  HTMLTableElement,
  React.HTMLAttributes<HTMLTableElement>
>(({ className, ...props }, ref) => (
  <div className="relative w-full overflow-auto">
    <table
      ref={ref}
      className={cn('w-full caption-bottom text-sm', className)}
      {...props}
    />
  </div>
))
Table.displayName = 'Table'

const TableHeader = React.forwardRef<
  HTMLTableSectionElement,
  React.HTMLAttributes<HTMLTableSectionElement>
>(({ className, ...props }, ref) => (
  <thead
    ref={ref}
    className={cn('[&_tr]:border-b hover:[&_tr]:bg-transparent', className)}
    {...props}
  />
))
TableHeader.displayName = 'TableHeader'

const TableBody = React.forwardRef<
  HTMLTableSectionElement,
  React.HTMLAttributes<HTMLTableSectionElement>
>(({ className, ...props }, ref) => (
  <tbody ref={ref} className={cn('', className)} {...props} />
))
TableBody.displayName = 'TableBody'

const TableFooter = React.forwardRef<
  HTMLTableSectionElement,
  React.HTMLAttributes<HTMLTableSectionElement>
>(({ className, ...props }, ref) => (
  <tfoot
    ref={ref}
    className={cn(
      'bg-muted/50 border-t font-medium last:[&>tr]:border-b-0',
      className
    )}
    {...props}
  />
))
TableFooter.displayName = 'TableFooter'

export type TableRowProps = React.HTMLAttributes<HTMLTableRowElement> & {
  button?: boolean
}

const TableRow = React.forwardRef<HTMLTableRowElement, TableRowProps>(
  ({ className, button, ...props }, ref) => (
    <tr
      ref={ref}
      className={cn(
        'data-[state=selected]:bg-muted border-b transition-colors hover:bg-[#FAFAFA]',
        {
          'cursor-pointer': button
        },
        className
      )}
      {...props}
    />
  )
)
TableRow.displayName = 'TableRow'

export type TableHeadProps = React.HTMLAttributes<HTMLTableCellElement> & {
  align?: 'left' | 'center' | 'right'
}

const TableHead = React.forwardRef<HTMLTableCellElement, TableHeadProps>(
  ({ className, align, ...props }, ref) => (
    <th
      ref={ref}
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
)
TableHead.displayName = 'TableHead'

const TableCell = React.forwardRef<
  HTMLTableCellElement,
  React.TdHTMLAttributes<HTMLTableCellElement>
>(({ className, ...props }, ref) => (
  <td
    ref={ref}
    className={cn(
      'text-shadcn-500 px-6 py-4 align-middle text-sm font-normal [&:has([role=checkbox])]:pr-0',
      className
    )}
    {...props}
  />
))
TableCell.displayName = 'TableCell'

const TableCaption = React.forwardRef<
  HTMLTableCaptionElement,
  React.HTMLAttributes<HTMLTableCaptionElement>
>(({ className, ...props }, ref) => (
  <caption
    ref={ref}
    className={cn('text-muted-foreground mt-4 text-sm', className)}
    {...props}
  />
))
TableCaption.displayName = 'TableCaption'

export {
  TableContainer,
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption
}
