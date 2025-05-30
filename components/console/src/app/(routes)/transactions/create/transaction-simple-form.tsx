'use client'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { ArrowRight } from 'lucide-react'
import { useIntl } from 'react-intl'
import { Stepper } from './components/stepper'
import { OperationAccordion } from './components/operation-accordion'
import { useTransactionForm } from './transaction-form-provider'
import { MetadataAccordion } from './components/metadata-accordion'
import { BasicInformationPaper } from './components/basic-information-paper'
import { StepperContent } from '@/components/ui/stepper'
import { TransactionModeButton } from '@/components/transactions/transaction-mode-button'
import { OperationSourceSimpleField } from './components/operation-source-simple-field'
import {
  FadeEffect,
  NextButton,
  SectionTitle,
  SideControl,
  SideControlActions,
  SideControlCancelButton,
  SideControlTitle
} from './primitives'
import { useTransactionMode } from './hooks/use-transaction-mode'

export type TransactionSimpleFormProps = {
  onModeClick?: () => void
  onCancel?: () => void
  onConfirm?: () => void
}

export const TransactionSimpleForm = ({
  onModeClick,
  onCancel
}: TransactionSimpleFormProps) => {
  const intl = useIntl()

  const { mode } = useTransactionMode()
  const {
    form,
    enableNext,
    currentStep,
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
                id: 'transactions.create.mode.simple',
                defaultMessage: 'New simple Transaction'
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
                <OperationSourceSimpleField
                  name="source"
                  onSubmit={addSource}
                  onRemove={removeSource}
                  control={form.control}
                  expand
                />
              </div>
              <div className="flex items-center justify-center">
                <ArrowRight className="h-5 w-5 shrink-0 text-shadcn-400" />
              </div>
              <div className="col-span-5 flex items-center justify-center">
                <OperationSourceSimpleField
                  name="destination"
                  onSubmit={addDestination}
                  onRemove={removeDestination}
                  control={form.control}
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

            <div className="mb-8">
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
