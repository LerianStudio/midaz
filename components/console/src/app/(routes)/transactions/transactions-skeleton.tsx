import { Skeleton } from '@/components/ui/skeleton'
import React from 'react'

export const TransactionsSkeleton = () => {
  return (
    <React.Fragment>
      <Skeleton className="h-[390px] w-full bg-zinc-200" />
    </React.Fragment>
  )
}
