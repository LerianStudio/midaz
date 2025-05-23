import { useIntl } from 'react-intl'
import {
  Stepper as PrimitiveStepper,
  StepperItem,
  StepperItemNumber,
  StepperItemText
} from '@/components/ui/stepper'

export type StepperProps = {
  step?: number
}

export const Stepper = ({ step = 0 }: StepperProps) => {
  const intl = useIntl()

  return (
    <PrimitiveStepper>
      <StepperItem active={step === 0}>
        <StepperItemNumber>1</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'transactions.create.stepper.first',
            defaultMessage: 'Transaction Data'
          })}
        />
      </StepperItem>
      <StepperItem active={step === 1}>
        <StepperItemNumber>2</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'transactions.create.stepper.second',
            defaultMessage: 'Operations and Metadata'
          })}
        />
      </StepperItem>
      <StepperItem active={step === 2}>
        <StepperItemNumber>3</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'transactions.create.stepper.third',
            defaultMessage: 'Review'
          })}
          description={intl.formatMessage({
            id: 'transactions.create.stepper.third.description',
            defaultMessage:
              'Check the values ​​and parameters entered and confirm to create the transaction.'
          })}
        />
      </StepperItem>
    </PrimitiveStepper>
  )
}
