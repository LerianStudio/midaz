import { Skeleton } from '@/components/ui/skeleton'
import React from 'react'

export const PortfoliosSkeleton = () => {
  return (
    <React.Fragment>
      <Skeleton className="h-[84px] w-full bg-zinc-200" />
      <Skeleton className="mt-2 h-[390px] w-full bg-zinc-200" />
    </React.Fragment>
  )
}
