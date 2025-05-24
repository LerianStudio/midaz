import React from 'react'
import { Badge } from '@/components/ui/badge'
import { CheckCircle, XCircle } from 'lucide-react'
import { cn } from '@/lib/utils'

interface PackageStatusBadgeProps {
  active: boolean
  className?: string
}

export function PackageStatusBadge({
  active,
  className
}: PackageStatusBadgeProps) {
  return (
    <Badge
      variant={active ? 'default' : 'secondary'}
      className={cn(
        'flex items-center gap-1',
        active
          ? 'bg-green-100 text-green-800 hover:bg-green-100'
          : 'bg-gray-100 text-gray-600',
        className
      )}
    >
      {active ? (
        <>
          <CheckCircle className="h-3 w-3" />
          Active
        </>
      ) : (
        <>
          <XCircle className="h-3 w-3" />
          Inactive
        </>
      )}
    </Badge>
  )
}
