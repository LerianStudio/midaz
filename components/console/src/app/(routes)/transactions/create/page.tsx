'use client'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { LoadingButton } from '@/components/ui/loading-button'
import { ArrowRight, Info } from 'lucide-react'
import { useIntl } from 'react-intl'
import { Stepper } from './stepper'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import Image from 'next/image'
import {
  OperationAccordion,
  OperationEmptyAccordion
} from './operation-accordion'
import { OperationSourceField } from './operation-source-field'
import { useTransactionForm } from './transaction-form-provider'
import { MetadataAccordion } from './metadata-accordion'
import ArrowRightCircle from '/public/svg/arrow-right-circle.svg'
import { BasicInformationPaper } from './basic-information-paper'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useRouter } from 'next/navigation'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { StepperContent } from '@/components/ui/stepper'

export default function CreateTransactionPage() {
  const intl = useIntl()
  const router = useRouter()

  const {
    form,
    currentStep,
    multipleSources,
    values,
    addSource,
    addDestination,
    handleReview
  } = useTransactionForm()

  const { handleDialogOpen, dialogProps } = useConfirmDialog({
    onConfirm: () => router.push('/transactions')
  })

  return (
    <>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'transaction.create.cancel.title',
          defaultMessage: 'Do you wish to cancel this transaction?'
        })}
        description={intl.formatMessage({
          id: 'transaction.create.cancel.description',
          defaultMessage:
            'If you cancel this transaction, all filled data will be lost and cannot be recovered.'
        })}
        {...dialogProps}
      />

      <Form {...form}>
        <div className="grid grid-cols-3">
          <div className="col-span-2">
            <BasicInformationPaper control={form.control} />

            <div className="mb-10 flex flex-row items-center gap-3">
              <OperationSourceField
                name="source"
                label={intl.formatMessage({
                  id: 'transactions.source',
                  defaultMessage: 'Source'
                })}
                values={values.source}
                onSubmit={addSource}
                control={form.control}
              />
              <Image alt="" src={ArrowRightCircle} />
              <OperationSourceField
                name="destination"
                label={intl.formatMessage({
                  id: 'transactions.destination',
                  defaultMessage: 'Destination'
                })}
                values={values.destination}
                onSubmit={addDestination}
                control={form.control}
              />
            </div>

            <StepperContent active={currentStep === 0}>
              <OperationEmptyAccordion
                title={intl.formatMessage({
                  id: 'common.operations',
                  defaultMessage: 'Operations'
                })}
                description={intl.formatMessage({
                  id: 'transactions.create.operations.accordion.description',
                  defaultMessage:
                    'Fill in Value, Source and Destination to edit the Operations.'
                })}
              />

              <OperationEmptyAccordion
                title={intl.formatMessage({
                  id: 'common.metadata',
                  defaultMessage: 'Metadata'
                })}
                description={intl.formatMessage({
                  id: 'transactions.create.metadata.accordion.description',
                  defaultMessage:
                    'Fill in Value, Source and Destination to edit the Metadata.'
                })}
              />
            </StepperContent>

            <StepperContent active={currentStep === 1}>
              <h6 className="mb-6 text-sm font-medium">
                {intl.formatMessage({
                  id: 'common.operations',
                  defaultMessage: 'Operations'
                })}
              </h6>

              {multipleSources && (
                <Alert className="mb-6" variant="informative">
                  <Info className="h-4 w-4" />
                  <AlertTitle>
                    {intl.formatMessage({
                      id: 'transactions.operations.alert.title',
                      defaultMessage: 'Multiple origins and destinations'
                    })}
                  </AlertTitle>
                  <AlertDescription>
                    {intl.formatMessage({
                      id: 'transactions.operations.alert.description',
                      defaultMessage:
                        'Fill in the value fields to adjust the amount transacted. Remember: the total Credits must equal the total Debits.'
                    })}
                  </AlertDescription>
                </Alert>
              )}

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
          </div>

          <div className="col-span-1">
            <div className="sticky top-12 ml-12 mt-4">
              <Stepper step={currentStep} />
            </div>
          </div>
        </div>

        <PageFooter open={currentStep > 0}>
          <PageFooterSection>
            <Button variant="outline" onClick={() => handleDialogOpen('')}>
              {intl.formatMessage({
                id: 'common.cancel',
                defaultMessage: 'Cancel'
              })}
            </Button>
          </PageFooterSection>
          <PageFooterSection>
            <LoadingButton
              icon={<ArrowRight />}
              iconPlacement="end"
              onClick={form.handleSubmit(handleReview)}
            >
              {intl.formatMessage({
                id: 'transactions.create.review.button',
                defaultMessage: 'Go to Review'
              })}
            </LoadingButton>
          </PageFooterSection>
        </PageFooter>
      </Form>
    </>
  )
}
