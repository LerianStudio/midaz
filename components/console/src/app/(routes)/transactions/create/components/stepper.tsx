import { useIntl } from 'react-intl'
import {
  Stepper as PrimitiveStepper,
  StepperItem,
  StepperItemNumber,
  StepperItemSkeleton,
  StepperItemText
} from '@/components/ui/stepper'

export const StepperSkeleton = () => (
  <div className="flex flex-col gap-4">
    <StepperItemSkeleton />
    <StepperItemSkeleton />
    <StepperItemSkeleton />
  </div>
)

export type StepperProps = {
  step?: number
}

export const Stepper = ({ step = 0 }: StepperProps) => {
  const intl = useIntl()

  return (
    <PrimitiveStepper>
      <StepperItem active={step === 0} complete={step > 0}>
        <StepperItemNumber>1</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'transactions.create.stepper.first',
            defaultMessage: 'Transaction Data'
          })}
          description={intl.formatMessage({
            id: 'transactions.create.stepper.first.description',
            defaultMessage: 'Fill in the basic transaction details.'
          })}
        />
      </StepperItem>
      <StepperItem active={step === 1} complete={step > 1}>
        <StepperItemNumber>2</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'transactions.create.stepper.second',
            defaultMessage: 'Source / Destination'
          })}
          description={intl.formatMessage({
            id: 'transactions.create.stepper.second.description',
            defaultMessage: 'Fill in the origin and destination data.'
          })}
        />
      </StepperItem>
      <StepperItem active={step === 2}>
        <StepperItemNumber>3</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'transactions.create.stepper.third',
            defaultMessage: 'Operations'
          })}
          description={intl.formatMessage({
            id: 'transactions.create.stepper.third.description',
            defaultMessage: 'Review transaction operations and metadata.'
          })}
        />
      </StepperItem>
    </PrimitiveStepper>
  )
}
