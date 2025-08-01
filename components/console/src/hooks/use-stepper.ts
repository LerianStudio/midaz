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
    if (maxSteps && step >= maxSteps - 1) {
      return
    }

    setStep((prev) => prev + 1)
  }

  const handlePrevious = () => {
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
