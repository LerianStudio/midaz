import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'

export const SkeletonTransactionDialog = () => {
  return (
    <div className="p-16">
      <PageHeader.Root>
        <PageHeader.Wrapper>
          <div className="mb-10 flex w-full items-center justify-between">
            <div className="space-y-2">
              <Skeleton className="h-10 w-[300px] bg-zinc-200" />
              <Skeleton className="h-5 w-[200px] bg-zinc-200" />
            </div>
            <Skeleton className="h-8 w-[100px] bg-zinc-200" />
          </div>
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <div className="mx-auto mt-10 max-w-[700px] space-y-4">
        <div className="space-y-4 rounded-lg border p-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <Skeleton className="h-12 w-12 rounded-full bg-zinc-200" />
              <Skeleton className="h-8 w-[200px] bg-zinc-200" />
            </div>
            <Skeleton className="h-8 w-[100px] bg-zinc-200" />
          </div>
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <Skeleton className="h-6 w-[150px] bg-zinc-200" />
              <Skeleton className="h-6 w-[150px] bg-zinc-200" />
            </div>
            <Skeleton className="h-20 w-full bg-zinc-200" />
          </div>
        </div>

        <div className="space-y-4 rounded-lg border p-6">
          <div className="space-y-6">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="flex items-center justify-between">
                <Skeleton className="h-4 w-[120px] bg-zinc-200" />
                <Skeleton className="h-4 w-[200px] bg-zinc-200" />
              </div>
            ))}
          </div>
          <Skeleton className="h-px w-full bg-zinc-200" />
          {[1, 2].map((i) => (
            <div key={i} className="space-y-2">
              <Skeleton className="h-6 w-[150px] bg-zinc-200" />
              <div className="flex items-center justify-between">
                <Skeleton className="h-4 w-[200px] bg-zinc-200" />
                <Skeleton className="h-4 w-[100px] bg-zinc-200" />
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
