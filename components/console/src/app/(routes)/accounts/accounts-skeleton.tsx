import { Skeleton } from '@/components/ui/skeleton'
import React from 'react'

export const AccountsSkeleton = () => {
  return (
    <React.Fragment>
      <Skeleton className="h-[84px] w-full bg-zinc-200" />
      <Skeleton className="mt-6 h-[390px] w-full bg-zinc-200" />
    </React.Fragment>
  )
}
