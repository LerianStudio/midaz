import { cn } from '@/lib/utils'
import { ReactNode } from 'react'

type InfoTitleProps = {
  title: string
  subtitle?: string
  subtitleCopyToClipboard?: boolean
  className?: string
  children?: ReactNode
}

export const InfoTitle = ({
  title,
  subtitle,
  className,
  children
}: InfoTitleProps) => (
  <div className="mb-12 flex flex-col gap-4">
    <h1
      className={cn('text-4xl font-bold text-[#3f3f46]', className)}
      data-testid="title"
    >
      {title}
    </h1>

    <div className="flex items-center gap-2">
      <p className="text-shadcn-400 text-sm font-medium">{subtitle}</p>
      {children}
    </div>
  </div>
)
