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

  useEffect(() => {
    if (step === 0) {
      setEnableNext(asset !== '' && value > 0)
    }
  }, [step, asset, value])

  useEffect(() => {
    if (step === 1) {
      setEnableNext(source?.length > 0 && destination?.length > 0)
    }
  }, [step, source?.length, destination?.length])

  useEffect(() => {
    if (step === 2) {
      if (source?.length === 0 || destination?.length === 0) {
        handlePrevious()
        return
      }

      setEnableNext(true)
    }
  }, [step, source?.length, destination?.length])

  return { step, setStep, handleNext, handlePrevious, enableNext, ...props }
}
