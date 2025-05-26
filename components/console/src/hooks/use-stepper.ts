import { useState } from 'react'

export type UseStepperProps = {
  defaultStep?: number
  maxSteps?: number
}

export const useStepper = ({
  defaultStep = 0,
  maxSteps = 2
}: UseStepperProps) => {
  const [step, setStep] = useState(defaultStep)

  const handleNext = () => {
    // If maxSteps is defined and the current step is the last step, do nothing
    if (maxSteps && step >= maxSteps - 1) {
      return
    }

    setStep((prev) => prev + 1)
  }

  const handlePrevious = () => {
    // If the current step is the first step, do nothing
    if (step <= 0) {
      return
    }

    setStep((prev) => prev - 1)
  }

  return {
    step,
    setStep,
    handleNext,
    handlePrevious
  }
}
