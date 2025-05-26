import { useEffect, useState } from 'react'
import { useStepper } from '@/hooks/use-stepper'
import { TransactionFormSchema } from '../schemas'

export const useTransactionFormControl = (values: TransactionFormSchema) => {
  const [enableNext, setEnableNext] = useState(false)
  const {
    step,
    setStep,
    handlePrevious,
    handleNext: _handleNext,
    ...props
  } = useStepper({
    maxSteps: 4
  })
  const { asset, value, source, destination } = values

  const handleNext = () => {
    if (enableNext) {
      setEnableNext(false)
      _handleNext()
    }
  }

  // Enable next if asset and value are filled
  useEffect(() => {
    if (step === 0) {
      setEnableNext(asset !== '' && value > 0)
    }
  }, [step, asset, value])

  // Enable next if source and destination are filled
  useEffect(() => {
    if (step === 1) {
      setEnableNext(source?.length > 0 && destination?.length > 0)
    }
  }, [step, source?.length, destination?.length])

  // Always enable next from last step
  useEffect(() => {
    if (step === 2) {
      // Return to last step if source or destination are empty
      if (source?.length === 0 || destination?.length === 0) {
        handlePrevious()
        return
      }

      setEnableNext(true)
    }
  }, [step, source?.length, destination?.length])

  return { step, setStep, handleNext, handlePrevious, enableNext, ...props }
}
