import { useEffect } from 'react'
import { useStepper } from '@/hooks/use-stepper'
import { TransactionFormSchema } from './schemas'

export const useTransactionFormControl = (values: TransactionFormSchema) => {
  const { step, setStep, ...props } = useStepper({ maxSteps: 3 })
  const { asset, value, source, destination } = values

  useEffect(() => {
    if (step < 2) {
      // If the user has filled the required fields, move to the next step
      if (
        value !== 0 &&
        asset !== '' &&
        source?.length > 0 &&
        destination?.length > 0
      ) {
        setStep(1)
        // Reset back to initial step if the user has removed the required fields
      } else {
        setStep(0)
      }
    }
  }, [value, asset, source, destination])

  return { step, setStep, ...props }
}
