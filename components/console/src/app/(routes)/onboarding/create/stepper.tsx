import {
  Stepper as PrimitiveStepper,
  StepperItem,
  StepperItemNumber,
  StepperItemText
} from '@/components/ui/stepper'
import { useIntl } from 'react-intl'
import { useOnboardForm } from './onboard-form-provider'

export const Stepper = () => {
  const intl = useIntl()

  const { step, setStep } = useOnboardForm()

  return (
    <PrimitiveStepper>
      <StepperItem
        active={step === 0}
        complete={step > 0}
        onClick={() => {
          if (step > 0) {
            setStep(0)
          }
        }}
      >
        <StepperItemNumber>1</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'onboarding.stepper.step1',
            defaultMessage: 'Org details'
          })}
          description={intl.formatMessage({
            id: 'onboarding.stepper.step1Description',
            defaultMessage: `To get started, complete your Organization's registration`
          })}
        />
      </StepperItem>
      <StepperItem
        active={step === 1}
        complete={step > 1}
        onClick={() => {
          if (step > 1) {
            setStep(1)
          }
        }}
      >
        <StepperItemNumber>2</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'onboarding.stepper.step2',
            defaultMessage: 'Address'
          })}
          description={intl.formatMessage({
            id: 'onboarding.stepper.step2Description',
            defaultMessage: `Now provide your Organization address`
          })}
        />
      </StepperItem>
      <StepperItem active={step === 2} complete={step > 2}>
        <StepperItemNumber>3</StepperItemNumber>
        <StepperItemText
          title={intl.formatMessage({
            id: 'onboarding.stepper.step3',
            defaultMessage: 'Theme'
          })}
          description={intl.formatMessage({
            id: 'onboarding.stepper.step3Description',
            defaultMessage: `Customize your Organization's UI (optional)`
          })}
        />
      </StepperItem>
    </PrimitiveStepper>
  )
}
