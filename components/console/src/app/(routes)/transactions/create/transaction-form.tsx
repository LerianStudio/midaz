import { TransactionModeButtonSkeleton } from '@/components/transactions/transaction-mode-button'
import { TransactionMode } from './hooks/use-transaction-mode'
import { SideControl, SideControlTitle } from './primitives'
import {
  TransactionComplexForm,
  TransactionComplexFormProps
} from './transaction-complex-form'
import {
  TransactionSimpleForm,
  TransactionSimpleFormProps
} from './transaction-simple-form'
import { Skeleton } from '@/components/ui/skeleton'
import { StepperSkeleton } from './components/stepper'

export type TransactionFormProps = TransactionSimpleFormProps &
  TransactionComplexFormProps & {
    mode: TransactionMode
  }

export const TransactionFormSkeleton = () => (
  <div className="grid h-full grid-cols-3 gap-4">
    <div className="col-span-1">
      <SideControl>
        <SideControlTitle>
          <Skeleton className="h-10" />
        </SideControlTitle>
        <TransactionModeButtonSkeleton />
        <StepperSkeleton />
      </SideControl>
    </div>

    <div className="relative col-span-2 overflow-y-auto py-16 pr-16">
      <Skeleton className="mb-24 mt-48 h-60 w-full rounded-lg bg-zinc-200" />
    </div>
  </div>
)

export function TransactionForm({ mode, ...props }: TransactionFormProps) {
  if (mode === TransactionMode.COMPLEX) {
    return <TransactionComplexForm {...props} />
  }

  return <TransactionSimpleForm {...props} />
}
