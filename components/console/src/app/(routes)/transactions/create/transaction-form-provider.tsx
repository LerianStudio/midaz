import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { createContext, PropsWithChildren, useContext } from 'react'
import { useFieldArray, UseFieldArrayReturn, useForm } from 'react-hook-form'
import { useTransactionFormControl } from './hooks/use-transaction-form-control'
import {
  initialValues,
  sourceInitialValues,
  transactionFormSchema,
  TransactionFormSchema,
  TransactionSourceFormSchema
} from './schemas'
import { useTransactionFormErrors } from './hooks/use-transaction-form-errors'
import { AccountDto } from '@/core/application/dto/account-dto'
import { BalanceDto } from '@/core/application/dto/balance-dto'
import { useAccounts } from './hooks/use-account'
import { CustomFormErrors } from '@/hooks/use-custom-form-error'
import { useIntl } from 'react-intl'
import { useTransactionMode } from './hooks/use-transaction-mode'

type TransactionFormProviderContext = {
  accounts: Record<string, AccountDto>
  form: ReturnType<typeof useForm<TransactionFormSchema>>
  errors: CustomFormErrors
  openFundsModal: boolean
  setOpenFundsModal: (open: boolean) => void
  currentStep: number
  enableNext: boolean
  multipleSources?: boolean
  values: TransactionFormSchema
  addBalance: (alias: string, balance: BalanceDto) => void
  addSource: (alias: string, account: AccountDto) => void
  removeSource: (alias: string) => void
  addDestination: (alias: string, account: AccountDto) => void
  removeDestination: (alias: string) => void
  handleNextStep: () => void
  handleReview: () => void
  handleForceReview: () => void
  handleBack: () => void
  handleReset: () => void
}

const TransactionFormProvider = createContext<TransactionFormProviderContext>(
  {} as never
)

export const useTransactionForm = () => {
  return useContext(TransactionFormProvider)
}

export type TransactionProviderProps = PropsWithChildren & {
  values?: TransactionFormSchema
}

export const TransactionProvider = ({
  values,
  children
}: TransactionProviderProps) => {
  const intl = useIntl()
  const {
    accounts,
    add: addAccount,
    clear: clearAccounts,
    addBalance
  } = useAccounts()

  const form = useForm<TransactionFormSchema>({
    resolver: zodResolver(transactionFormSchema) as any,
    values,
    defaultValues: initialValues as any
  })

  const formValues = form.watch()

  const { mode } = useTransactionMode()
  const { step, setStep, enableNext, handleNext, handlePrevious } =
    useTransactionFormControl(formValues)
  const {
    errors,
    add: addError,
    validate,
    open: openFundsModal,
    setOpen: setOpenFundsModal
  } = useTransactionFormErrors(formValues, accounts)

  const originFieldArray = useFieldArray({
    name: 'source',
    control: form.control
  })

  const destinationFieldArray = useFieldArray({
    name: 'destination',
    control: form.control
  })

  // Flag to represent if the transaction has multiple sources or destinations
  const multipleSources =
    originFieldArray.fields.length > 1 ||
    destinationFieldArray.fields.length > 1

  // Add source or destination to the transaction
  // The first entity uses the same value as the transaction
  // Latter ones will start at 0
  const addSource = (
    fieldArray: UseFieldArrayReturn<any>,
    alias: string,
    account: AccountDto
  ) => {
    if (fieldArray.fields.length === 0) {
      fieldArray.append({
        ...sourceInitialValues,
        accountAlias: alias,
        value: formValues.value
      })
      addAccount(alias, account)
      return
    }

    const index = fieldArray.fields.findIndex(
      (item) =>
        (item as unknown as TransactionSourceFormSchema[0]).accountAlias ===
        alias
    )
    if (index > -1) {
      addError('search', {
        message: intl.formatMessage(
          {
            id: 'transactions.errors.account.duplicate',
            defaultMessage: 'Account {alias} already exists'
          },
          { alias }
        )
      })
      return
    }

    fieldArray.append({
      ...sourceInitialValues,
      accountAlias: alias
    })
    addAccount(alias, account)
  }

  const removeSource = (
    fieldArray: UseFieldArrayReturn<any>,
    alias: string
  ) => {
    if (fieldArray.fields.length === 0) {
      return
    }

    const index = fieldArray.fields.findIndex(
      (item) =>
        (item as unknown as TransactionSourceFormSchema[0]).accountAlias ===
        alias
    )

    if (index > -1) {
      fieldArray.remove(index)
      return
    }
  }

  const handleReset = () => {
    form.reset()
    setStep(0)
    clearAccounts()
  }

  const handleReview = form.handleSubmit(validate(() => handleNext()))

  const handleForceReview = () => {
    handleNext()
  }

  // In case the user adds more than 1 source or destination,
  // And then removes to stay with only 1, we need to restore the original
  // transaction value to the source or destination
  useEffect(() => {
    if (formValues.source.length === 1) {
      form.setValue('source.0.value', formValues.value)
    }
  }, [formValues.value, formValues.source.length])
  useEffect(() => {
    if (formValues.destination.length === 1) {
      form.setValue('destination.0.value', formValues.value)
    }
  }, [formValues.value, formValues.destination.length])

  // Downgrade the data if we are moving from complex to simple mode
  // This is important, or else the user could send information that is not
  // present on the screen
  useEffect(() => {
    if (mode === 'simple') {
      if (formValues.source.length > 1) {
        form.setValue('source', [formValues.source[0]])
      }

      if (formValues.destination.length > 1) {
        form.setValue('destination', [formValues.destination[0]])
      }
    }
  }, [mode])

  return (
    <TransactionFormProvider.Provider
      value={{
        accounts,
        form,
        errors,
        openFundsModal,
        setOpenFundsModal,
        currentStep: step,
        multipleSources,
        values: formValues,
        addBalance,
        addSource: (alias: string, account: AccountDto) =>
          addSource(originFieldArray, alias, account),
        removeSource: (alias: string) => removeSource(originFieldArray, alias),
        addDestination: (alias: string, account: AccountDto) =>
          addSource(destinationFieldArray, alias, account),
        removeDestination: (alias: string) =>
          removeSource(destinationFieldArray, alias),
        enableNext,
        handleNextStep: handleNext,
        handleBack: handlePrevious,
        handleReview,
        handleForceReview,
        handleReset
      }}
    >
      {children}
    </TransactionFormProvider.Provider>
  )
}
