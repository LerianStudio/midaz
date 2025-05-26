'use client'

import { useTransactionForm } from './transaction-form-provider'
import { Form } from '@/components/ui/form'
import {
  FadeEffect,
  NextButton,
  SectionTitle,
  SideControl,
  SideControlActions,
  SideControlCancelButton,
  SideControlTitle
} from './primitives'
import { useIntl } from 'react-intl'
import { Stepper } from './components/stepper'
import { BasicInformationPaper } from './components/basic-information-paper'
import { StepperContent } from '@/components/ui/stepper'
import { ArrowRight, Info } from 'lucide-react'
import { OperationAccordion } from './components/operation-accordion'
import { MetadataAccordion } from './components/metadata-accordion'
import { Button } from '@/components/ui/button'
import { AccountBalanceList } from './components/account-balance-list'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { AccountSearchField } from './components/account-search-field'
import { OperationSum } from './components/operation-sum'
import { useTransactionMode } from './hooks/use-transaction-mode'
import { TransactionModeButton } from '@/components/transactions/transaction-mode-button'

export type TransactionComplexFormProps = {
  onModeClick?: () => void
  onCancel?: () => void
  onConfirm?: () => void
}

export const TransactionComplexForm = ({
  onModeClick,
  onCancel
}: TransactionComplexFormProps) => {
  const intl = useIntl()
  const { mode } = useTransactionMode()
  const {
    form,
    errors,
    enableNext,
    currentStep,
    multipleSources,
    values,
    addSource,
    removeSource,
    addDestination,
    removeDestination,
    handleNextStep,
    handleReview
  } = useTransactionForm()

  return (
    <Form {...form}>
      <div className="grid h-full grid-cols-3 gap-4">
        <div className="col-span-1">
          <SideControl>
            <SideControlTitle>
              {intl.formatMessage({
                id: 'transactions.create.mode.complex',
                defaultMessage: 'New complex Transaction'
              })}
            </SideControlTitle>
            <TransactionModeButton
              className="mb-7"
              mode={mode}
              onChange={onModeClick}
            />
            <Stepper step={currentStep} />
            <SideControlActions>
              <SideControlCancelButton onClick={onCancel}>
                {intl.formatMessage({
                  id: 'common.cancel',
                  defaultMessage: 'Cancel'
                })}
              </SideControlCancelButton>
            </SideControlActions>
          </SideControl>
        </div>

        <div className="relative col-span-2 overflow-y-auto py-16 pr-16">
          <FadeEffect />
          <SectionTitle className="my-8">
            {intl.formatMessage({
              id: 'transactions.create.basicInformation.title',
              defaultMessage: 'Transaction Data'
            })}
          </SectionTitle>

          <BasicInformationPaper className="mb-24" control={form.control} />

          <StepperContent active={currentStep >= 1}>
            <div className="mb-24 grid grid-cols-11 gap-x-4">
              <div className="col-span-5 mb-8 flex flex-grow flex-col gap-1">
                <SectionTitle>
                  {intl.formatMessage({
                    id: 'entity.transactions.source',
                    defaultMessage: 'Source'
                  })}
                </SectionTitle>
                <p className="text-sm font-medium text-zinc-500">
                  {intl.formatMessage({
                    id: 'transactions.create.source.description',
                    defaultMessage: 'Which account will this amount come from?'
                  })}
                </p>
              </div>
              <div className="col-span-5 col-start-7 mb-8 flex flex-grow flex-col gap-1">
                <SectionTitle>
                  {intl.formatMessage({
                    id: 'entity.transactions.destination',
                    defaultMessage: 'Destination'
                  })}
                </SectionTitle>
                <p className="text-sm font-medium text-zinc-500">
                  {intl.formatMessage({
                    id: 'transactions.create.destination.description',
                    defaultMessage: 'Which account will receive this amount?'
                  })}
                </p>
              </div>

              <div className="col-span-5 flex items-center justify-center">
                <AccountSearchField
                  className="mb-4"
                  errors={errors}
                  onSelect={addSource}
                />
              </div>
              <div className="col-span-5 col-start-7 flex items-center justify-center">
                <AccountSearchField
                  className="mb-4"
                  onSelect={addDestination}
                />
              </div>
              <div className="col-span-5">
                <AccountBalanceList
                  name="source"
                  control={form.control}
                  onRemove={removeSource}
                  expand
                />
              </div>
              <div className="flex items-center justify-center">
                {(values.source?.length > 0 ||
                  values.destination?.length > 0) && (
                  <ArrowRight className="mb-14 h-5 w-5 shrink-0 text-shadcn-400" />
                )}
              </div>
              <div className="col-span-5">
                <AccountBalanceList
                  name="destination"
                  control={form.control}
                  onRemove={removeDestination}
                />
              </div>
            </div>
          </StepperContent>

          <StepperContent active={currentStep >= 2}>
            <SectionTitle className="mb-4">
              {intl.formatMessage({
                id: 'common.operations',
                defaultMessage: 'Operations'
              })}
            </SectionTitle>

            {multipleSources && (
              <Alert className="mb-6" variant="informative">
                <Info className="h-4 w-4" />
                <AlertTitle>
                  {intl.formatMessage({
                    id: 'transactions.operations.alert.title',
                    defaultMessage:
                      'Distribution between origins and destinations'
                  })}
                </AlertTitle>
                <AlertDescription>
                  {intl.formatMessage({
                    id: 'transactions.operations.alert.description',
                    defaultMessage:
                      'Enter the amounts to be debited from the source accounts and credited to the destination accounts. The Debit and Credit sums must match each other and the total transaction amount.'
                  })}
                </AlertDescription>
              </Alert>
            )}
            <div className="mb-8">
              <div className="">
                {values.source?.map((source, index) => (
                  <OperationAccordion
                    key={index}
                    name={`source.${index}`}
                    asset={values.asset}
                    values={source}
                    valueEditable={values.source.length > 1}
                    control={form.control}
                  />
                ))}
                <OperationSum
                  label={intl.formatMessage({
                    id: 'transactions.create.source.sum',
                    defaultMessage: 'Total Debit'
                  })}
                  errorMessage={intl.formatMessage({
                    id: 'transactions.errors.debit',
                    defaultMessage:
                      'The sum of the debits differs from the transaction amount'
                  })}
                  value={values.value}
                  asset={values.asset}
                  operations={values.source ?? []}
                />
              </div>
              {values.destination?.map((destination, index) => (
                <OperationAccordion
                  key={index}
                  type="credit"
                  name={`destination.${index}`}
                  asset={values.asset}
                  values={destination}
                  valueEditable={values.destination.length > 1}
                  control={form.control}
                />
              ))}
              <OperationSum
                label={intl.formatMessage({
                  id: 'transactions.create.destination.sum',
                  defaultMessage: 'Total Credit'
                })}
                errorMessage={intl.formatMessage({
                  id: 'transactions.errors.credit',
                  defaultMessage:
                    'The sum of the credits differs from the transaction amount'
                })}
                value={values.value}
                asset={values.asset}
                operations={values.destination ?? []}
              />
            </div>

            <MetadataAccordion
              name="metadata"
              values={values.metadata ?? {}}
              control={form.control}
            />
          </StepperContent>

          <div className="mb-56 flex justify-end">
            <StepperContent active={currentStep < 2}>
              <NextButton disabled={!enableNext} onClick={handleNextStep} />
            </StepperContent>
            <StepperContent active={currentStep === 2}>
              <Button
                icon={<ArrowRight />}
                iconPlacement="end"
                onClick={handleReview}
              >
                {intl.formatMessage({
                  id: 'transactions.create.review.button',
                  defaultMessage: 'Go to Review'
                })}
              </Button>
            </StepperContent>
          </div>
        </div>
      </div>
    </Form>
  )
}
