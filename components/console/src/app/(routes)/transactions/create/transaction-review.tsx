import { PageFooter, PageFooterSection } from '@/components/page-footer'
import {
  TransactionReceipt,
  TransactionReceiptValue,
  TransactionReceiptSubjects,
  TransactionReceiptDescription,
  TransactionReceiptItem,
  TransactionReceiptOperation,
  TransactionReceiptTicket
} from '@/components/transactions/primitives/transaction-receipt'
import { Button } from '@/components/ui/button'
import { LoadingButton } from '@/components/ui/loading-button'
import { isNil } from 'lodash'
import {
  ArrowLeftCircle,
  GitCompare,
  GitFork,
  SendHorizonal
} from 'lucide-react'
import { useTransactionForm } from './transaction-form-provider'
import { useIntl } from 'react-intl'
import { Separator } from '@/components/ui/separator'
import { useCreateTransaction } from '@/client/transactions'
import { useOrganization } from '@lerianstudio/console-layout'
import { useToast } from '@/hooks/use-toast'
import { useRouter } from 'next/navigation'
import { useState, useEffect } from 'react'
import { TransactionFormSchema } from './schemas'
import {
  TransactionMode,
  useTransactionMode
} from './hooks/use-transaction-mode'
import { useFeeCalculation } from './hooks/use-fee-calculation'
import { validateTransactionPreflight } from '@/utils/transaction-validation'

export const TransactionReview = () => {
  const intl = useIntl()
  const router = useRouter()
  const { toast } = useToast()
  const { currentOrganization, currentLedger } = useOrganization()
  const { mode } = useTransactionMode()
  const { values, handleBack, handleReset } = useTransactionForm()

  const [sendAnother, setSendAnother] = useState(false)

  const {
    mutate: calculateFees,
    isPending: calculatingFees,
    data: calculatedFees,
    error: feesError
  } = useFeeCalculation({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id!
  })

  const hasCalculatedFees =
    calculatedFees !== undefined || feesError !== undefined

  const { mutate: createTransaction, isPending: loading } =
    useCreateTransaction({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id!,
      onSuccess: (data) => {
        toast({
          description: intl.formatMessage({
            id: 'success.transactions.create',
            defaultMessage: 'Transaction created successfully'
          }),
          variant: 'success'
        })

        if (sendAnother) {
          handleReset()
          return
        }

        if (data.id) {
          router.push(`/transactions/${data.id}`)
        } else {
          router.push('/transactions')
        }
      }
    })

  useEffect(() => {
    if (values && currentOrganization.id && currentLedger.id) {
      const feesEnabled = process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED === 'true'

      if (feesEnabled) {
        calculateFees({ transaction: values })
      }
    }
  }, [values, currentOrganization.id, currentLedger.id])

  const parse = ({ value, ...values }: TransactionFormSchema) => ({
    ...values,
    amount: value.toString(),
    source: values.source?.map(({ value, ...source }) => ({
      ...source,
      amount: value.toString()
    })),
    destination: values.destination?.map(({ value, ...destination }) => ({
      ...destination,
      amount: value.toString()
    }))
  })

  const _isDeductibleFrom = calculatedFees?.transaction?.isDeductibleFrom

  const getTransactionPayload = () => {
    if (calculatedFees && calculatedFees.transaction) {
      const feeTransaction = calculatedFees.transaction
      const originalAmount = values.value?.toString() || '0'

      const sourceMap = new Map<string, any>()

      feeTransaction.send.source.from.forEach((source: any) => {
        const accountAlias = source.accountAlias

        if (sourceMap.has(accountAlias)) {
          const existing = sourceMap.get(accountAlias)
          const existingAmount = parseFloat(existing.amount.value)
          const sourceAmount = parseFloat(source.amount.value)
          const totalAmount = existingAmount + sourceAmount

          sourceMap.set(accountAlias, {
            ...existing,
            amount: {
              ...existing.amount,
              value: totalAmount.toString()
            },
            metadata: {
              ...existing.metadata,
              consolidatedOperations: true,
              originalAmounts: [
                ...(existing.metadata.originalAmounts || [
                  existing.amount.value
                ]),
                source.amount.value
              ]
            }
          })
        } else {
          sourceMap.set(accountAlias, {
            accountAlias: source.accountAlias,
            asset: source.amount.asset,
            amount: source.amount, // Keep the full amount object
            description: source.description,
            chartOfAccounts: source.chartOfAccounts,
            metadata: source.metadata || {}
          })
        }
      })

      const sourceOperations = Array.from(sourceMap.values())

      const allDestOperations = feeTransaction.send.distribute.to || []

      const feeOperations = allDestOperations.filter(
        (op: any) => op.metadata?.source
      )
      const nonFeeOperations = allDestOperations.filter(
        (op: any) => !op.metadata?.source
      )

      const accountsWithFees = new Set(
        feeOperations.map((op: any) => op.accountAlias)
      )
      const accountsWithPrincipal = new Set(
        nonFeeOperations.map((op: any) => op.accountAlias)
      )
      const _overlappingAccounts = new Set(
        [...accountsWithFees].filter((acc) => accountsWithPrincipal.has(acc))
      )

      // Keep fee operations separate from principal operations
      const destinationOperations: any[] = []

      nonFeeOperations.forEach((op: any) => {
        destinationOperations.push({
          accountAlias: op.accountAlias,
          asset: op.amount.asset,
          amount: op.amount, // Keep the full amount object
          description: op.description,
          chartOfAccounts: op.chartOfAccounts,
          metadata: op.metadata || {}
        })
      })

      feeOperations.forEach((feeOp: any) => {
        // Find the matching fee rule to get the proper fee label
        const matchingRule = calculatedFees?.transaction?.feeRules?.find(
          (rule: any) =>
            rule.creditAccount === feeOp.accountAlias ||
            rule.creditAccount.replace('@', '') ===
              feeOp.accountAlias.replace('@', '')
        )

        const feeLabel = matchingRule?.feeLabel || feeOp.description || 'Fee'

        destinationOperations.push({
          accountAlias: feeOp.accountAlias,
          asset: feeOp.amount.asset,
          amount: feeOp.amount,
          description: feeLabel,
          chartOfAccounts: feeOp.chartOfAccounts,
          metadata: {
            ...feeOp.metadata,
            isFee: true // Mark as fee operation
          }
        })
      })

      const consolidatedData = {
        sourceOperations,
        destinationOperations
      }

      const transactionTotal = sourceOperations.reduce((sum, op) => {
        return sum + parseFloat(op.amount.value)
      }, 0)

      if (transactionTotal.toString() !== originalAmount) {
        console.warn(
          '[TransactionReview] Source total does not match original amount:',
          {
            originalAmount,
            sourceTotal: transactionTotal,
            difference: transactionTotal - parseFloat(originalAmount)
          }
        )
      }

      // This ensures source total = destination total = transaction amount
      const totalTransactionAmount = transactionTotal.toString()

      return {
        description: feeTransaction.description,
        chartOfAccountsGroupName: feeTransaction.chartOfAccountsGroupName,
        amount: totalTransactionAmount, // Use the total amount including fees
        asset: feeTransaction.send.asset,
        source: sourceOperations.map((op) => ({
          accountAlias: op.accountAlias,
          asset: op.asset,
          amount: op.amount.value, // Extract just the value for API
          description: op.description,
          chartOfAccounts: op.chartOfAccounts,
          metadata: op.metadata?.source ? { source: op.metadata.source } : {}
        })),
        destination: destinationOperations.map((op) => ({
          accountAlias: op.accountAlias,
          asset: op.asset,
          amount: op.amount.value, // Extract just the value for API
          description: op.description,
          chartOfAccounts: op.chartOfAccounts,
          metadata: op.metadata?.source ? { source: op.metadata.source } : {}
        })),
        metadata: {
          ...(feeTransaction.metadata || {}),
          originalTransactionAmount: originalAmount,
          deductibleAccounts: (feeTransaction.feeRules || [])
            .filter((rule: any) => rule.isDeductibleFrom)
            .map((rule: any) => rule.creditAccount)
            .join(',')
        },
        _consolidatedData: consolidatedData
      }
    }
    return parse(values)
  }

  const handleSubmitAnother = () => {
    const payload = getTransactionPayload()

    const validationResult = validateTransactionPreflight(
      values,
      calculatedFees,
      payload
    )

    if (!validationResult.isValid) {
      toast({
        title: intl.formatMessage({
          id: 'transactions.validation.failed.title',
          defaultMessage: 'Transaction validation failed'
        }),
        description: validationResult.errors.join(', '),
        variant: 'destructive'
      })
      return
    }

    if (validationResult.warnings.length > 0) {
      console.warn('Transaction warnings:', validationResult.warnings)
    }

    setSendAnother(true)
    const { _consolidatedData, ...payloadForApi } = payload as any
    createTransaction(payloadForApi)
  }

  const handleSubmit = () => {
    const payload = getTransactionPayload()

    const validationResult = validateTransactionPreflight(
      values,
      calculatedFees,
      payload
    )

    if (!validationResult.isValid) {
      toast({
        title: intl.formatMessage({
          id: 'transactions.validation.failed.title',
          defaultMessage: 'Transaction validation failed'
        }),
        description: validationResult.errors.join(', '),
        variant: 'destructive'
      })
      return
    }

    if (validationResult.warnings.length > 0) {
      console.warn('Transaction warnings:', validationResult.warnings)
    }

    setSendAnother(false)
    const { _consolidatedData, ...payloadForApi } = payload as any
    createTransaction(payloadForApi)
  }

  return (
    <div className="px-24 py-8">
      <h6 className="text-shadcn-400 mb-4 text-sm font-medium">
        {mode === TransactionMode.SIMPLE &&
          intl.formatMessage({
            id: 'transactions.create.mode.simple',
            defaultMessage: 'New simple Transaction'
          })}
        {mode === TransactionMode.COMPLEX &&
          intl.formatMessage({
            id: 'transactions.create.mode.complex',
            defaultMessage: 'New complex Transaction'
          })}
      </h6>

      <div className="flex flex-col gap-6">
        <h1 className="py-9 text-4xl font-bold text-zinc-700">
          {intl.formatMessage({
            id: 'transactions.create.review.title',
            defaultMessage: 'Review and Submit Transaction'
          })}
        </h1>
        <div className="relative mb-8 flex flex-row items-center">
          <div className="absolute flex flex-row items-center gap-4">
            <Button
              variant="plain"
              className="p-0 text-zinc-300"
              onClick={handleBack}
            >
              <ArrowLeftCircle className="h-8 w-8" strokeWidth={1} />
            </Button>
            <p className="text-sm font-medium text-zinc-700">
              {intl.formatMessage({
                id: 'transactions.create.review.backButton',
                defaultMessage: 'Review'
              })}
            </p>
          </div>
          <p className="grow py-2 text-center text-sm font-medium text-zinc-500">
            {intl.formatMessage({
              id: 'transactions.create.review.description',
              defaultMessage:
                'Check the values ​​and parameters entered and confirm to send the transaction.'
            })}
          </p>
        </div>
      </div>

      <div className="mb-24 grid grid-cols-5">
        <div className="mr-12"></div>

        <div className="col-span-3 col-start-2">
          <TransactionReceipt className="mb-2 py-5">
            {mode === 'simple' && (
              <GitCompare
                className="h-9 w-9 -scale-x-100 rotate-90 text-zinc-400"
                strokeWidth={1}
              />
            )}
            {mode === 'complex' && (
              <GitFork
                className="h-9 w-9 rotate-90 text-zinc-400"
                strokeWidth={1}
              />
            )}
            <TransactionReceiptValue
              asset={values.asset}
              value={intl.formatNumber(values.value, {
                roundingPriority: 'morePrecision'
              })}
              finalAmount={
                hasCalculatedFees && !feesError && calculatedFees?.transaction
                  ? intl.formatNumber(
                      (() => {
                        const feeOps =
                          calculatedFees.transaction.send?.distribute?.to?.filter(
                            (dest: any) => dest.metadata?.source
                          ) || []

                        let nonDeductibleFees = 0
                        feeOps.forEach((op: any) => {
                          const rule =
                            calculatedFees.transaction.feeRules?.find(
                              (r: any) =>
                                r.creditAccount === op.accountAlias ||
                                r.creditAccount.replace('@', '') ===
                                  op.accountAlias.replace('@', '')
                            )
                          if (rule && !rule.isDeductibleFrom) {
                            nonDeductibleFees += parseFloat(op.amount.value)
                          }
                        })

                        return Number(values.value) + nonDeductibleFees
                      })(),
                      { roundingPriority: 'morePrecision' }
                    )
                  : undefined
              }
              isCalculatingFees={calculatingFees}
            />
            <TransactionReceiptSubjects
              sources={values.source?.map((source) => source.accountAlias)}
              destinations={values.destination?.map(
                (source) => source.accountAlias
              )}
            />
            {values.description && (
              <TransactionReceiptDescription>
                {values.description}
              </TransactionReceiptDescription>
            )}
          </TransactionReceipt>

          <TransactionReceipt type="ticket">
            <TransactionReceiptItem
              label={intl.formatMessage({
                id: 'entity.transactions.source',
                defaultMessage: 'Source'
              })}
              value={
                <div className="flex flex-col">
                  {values.source?.map((source, index) => (
                    <p key={index} className="underline">
                      {source.accountAlias}
                    </p>
                  ))}
                </div>
              }
            />
            <TransactionReceiptItem
              label={intl.formatMessage({
                id: 'entity.transactions.destination',
                defaultMessage: 'Destination'
              })}
              value={
                <div className="flex flex-col">
                  {values.destination?.map((destination, index) => (
                    <p key={index} className="underline">
                      {destination.accountAlias}
                    </p>
                  ))}
                </div>
              }
            />
            <TransactionReceiptItem
              label={intl.formatMessage({
                id: 'transactions.originalAmount',
                defaultMessage: 'Original amount'
              })}
              value={`${values.asset} ${intl.formatNumber(values.value, { roundingPriority: 'morePrecision' })}`}
            />

            {hasCalculatedFees && feesError && (
              <div className="mx-8 rounded-lg border border-red-200 bg-red-50 px-4 py-3 transition-all duration-300">
                <div className="flex items-center gap-2">
                  <div>
                    <p className="text-sm font-medium text-red-800">
                      {intl.formatMessage({
                        id: 'transactions.fees.error.title',
                        defaultMessage: 'Fee Calculation Failed'
                      })}
                    </p>
                    <p className="mt-1 text-xs text-red-500">
                      {intl.formatMessage({
                        id: 'transactions.fees.error.fallback',
                        defaultMessage:
                          'Transaction will proceed without fee calculation.'
                      })}
                    </p>
                  </div>
                </div>
              </div>
            )}

            <Separator orientation="horizontal" />
            {/* Show fee-adjusted amounts if available, otherwise show original values */}
            {calculatedFees && calculatedFees.transaction ? (
              <>
                {(() => {
                  const sourceOps = calculatedFees.transaction.send.source.from

                  const sourceMap = new Map<
                    string,
                    { amount: number; asset: string }
                  >()
                  sourceOps.forEach((source: any) => {
                    const current = sourceMap.get(source.accountAlias)
                    if (current) {
                      sourceMap.set(source.accountAlias, {
                        amount:
                          current.amount + parseFloat(source.amount.value),
                        asset: source.amount.asset
                      })
                    } else {
                      sourceMap.set(source.accountAlias, {
                        amount: parseFloat(source.amount.value),
                        asset: source.amount.asset
                      })
                    }
                  })

                  return Array.from(sourceMap.entries()).map(
                    ([accountAlias, data], index) => (
                      <TransactionReceiptOperation
                        key={`source-${index}`}
                        type="debit"
                        account={accountAlias}
                        asset={data.asset}
                        value={intl.formatNumber(data.amount, {
                          roundingPriority: 'morePrecision'
                        })}
                      />
                    )
                  )
                })()}
                {(() => {
                  const allDestinations =
                    calculatedFees.transaction.send.distribute.to
                  const nonFeeDestinations = allDestinations.filter(
                    (dest: any) => !dest.metadata?.source
                  )
                  const feeDestinations = allDestinations.filter(
                    (dest: any) => dest.metadata?.source
                  )

                  // For N:N transactions, split the original amount equally among non-fee destinations
                  const recipientCount = nonFeeDestinations.length
                  const amountPerRecipient =
                    recipientCount > 0
                      ? parseFloat(values.value.toString()) / recipientCount
                      : 0

                  const principalOperations = nonFeeDestinations.map(
                    (dest: any, index: number) => (
                      <TransactionReceiptOperation
                        key={`dest-principal-${index}`}
                        type="credit"
                        account={dest.accountAlias}
                        asset={dest.amount.asset}
                        value={intl.formatNumber(amountPerRecipient, {
                          roundingPriority: 'morePrecision'
                        })}
                      />
                    )
                  )

                  const feeOperations = feeDestinations.map(
                    (fee: any, index: number) => (
                      <TransactionReceiptOperation
                        key={`dest-fee-${index}`}
                        type="credit"
                        account={fee.accountAlias}
                        asset={fee.amount.asset}
                        value={intl.formatNumber(parseFloat(fee.amount.value), {
                          roundingPriority: 'morePrecision'
                        })}
                      />
                    )
                  )

                  return [...principalOperations, ...feeOperations]
                })()}
              </>
            ) : (
              <>
                {values.source?.map((source, index) => (
                  <TransactionReceiptOperation
                    key={index}
                    type="debit"
                    account={source.accountAlias}
                    asset={values.asset}
                    value={intl.formatNumber(source.value, {
                      roundingPriority: 'morePrecision'
                    })}
                  />
                ))}
                {values.destination?.map((destination, index) => (
                  <TransactionReceiptOperation
                    key={index}
                    type="credit"
                    account={destination.accountAlias}
                    asset={values.asset}
                    value={intl.formatNumber(destination.value, {
                      roundingPriority: 'morePrecision'
                    })}
                  />
                ))}
              </>
            )}
            <Separator orientation="horizontal" />

            <TransactionReceiptItem
              label={intl.formatMessage({
                id: 'transactions.create.field.chartOfAccountsGroupName',
                defaultMessage: 'Accounting route group'
              })}
              value={
                !isNil(values.chartOfAccountsGroupName) &&
                values.chartOfAccountsGroupName !== ''
                  ? values.chartOfAccountsGroupName
                  : intl.formatMessage({
                      id: 'common.none',
                      defaultMessage: 'None'
                    })
              }
            />
            <TransactionReceiptItem
              label={intl.formatMessage({
                id: 'common.metadata',
                defaultMessage: 'Metadata'
              })}
              value={intl.formatMessage(
                {
                  id: 'common.table.metadata',
                  defaultMessage:
                    '{number, plural, =0 {-} one {# record} other {# records}}'
                },
                {
                  number: Object.keys(values.metadata ?? {}).length
                }
              )}
            />

            {/* Fee Breakdown Section */}
            {hasCalculatedFees &&
              !feesError &&
              calculatedFees &&
              calculatedFees.transaction && (
                <>
                  <Separator orientation="horizontal" />
                  {/* Show individual fees grouped by account */}
                  {(() => {
                    const feeOps =
                      calculatedFees.transaction.send?.distribute?.to?.filter(
                        (dest: any) => dest.metadata?.source
                      ) || []

                    const feesByAccount = new Map<
                      string,
                      {
                        total: number
                        operations: any[]
                        rule: any
                        asset: string
                      }
                    >()

                    feeOps.forEach((feeOp: any) => {
                      const matchingRule =
                        calculatedFees.transaction.feeRules?.find(
                          (rule: any) =>
                            rule.creditAccount === feeOp.accountAlias ||
                            rule.creditAccount.replace('@', '') ===
                              feeOp.accountAlias.replace('@', '')
                        )

                      const current = feesByAccount.get(feeOp.accountAlias)
                      if (current) {
                        current.total += parseFloat(feeOp.amount.value)
                        current.operations.push(feeOp)
                      } else {
                        feesByAccount.set(feeOp.accountAlias, {
                          total: parseFloat(feeOp.amount.value),
                          operations: [feeOp],
                          rule: matchingRule,
                          asset: feeOp.amount.asset
                        })
                      }
                    })

                    return Array.from(feesByAccount.entries()).map(
                      ([accountAlias, data], index) => {
                        const label =
                          data.rule?.feeLabel ||
                          data.operations[0]?.description ||
                          `Fee - ${accountAlias}`

                        return (
                          <TransactionReceiptItem
                            key={index}
                            label={label}
                            value={
                              <div className="flex items-center gap-2">
                                <span className="text-blue-600">
                                  +{data.asset}{' '}
                                  {intl.formatNumber(data.total, {
                                    roundingPriority: 'morePrecision'
                                  })}
                                </span>
                                {data.operations.length > 1 && (
                                  <span className="text-xs text-gray-500">
                                    ({data.operations.length} operations)
                                  </span>
                                )}
                              </div>
                            }
                          />
                        )
                      }
                    )
                  })()}

                  <Separator orientation="horizontal" />

                  {/* Source pays / Destination receives */}
                  {(() => {
                    const feeOps =
                      calculatedFees.transaction.send?.distribute?.to?.filter(
                        (dest: any) => dest.metadata?.source
                      ) || []

                    const feesByAccount = new Map<
                      string,
                      { total: number; isDeductible: boolean }
                    >()

                    feeOps.forEach((op: any) => {
                      const rule = calculatedFees.transaction.feeRules?.find(
                        (r: any) =>
                          r.creditAccount === op.accountAlias ||
                          r.creditAccount.replace('@', '') ===
                            op.accountAlias.replace('@', '')
                      )

                      if (rule) {
                        const current = feesByAccount.get(op.accountAlias)
                        if (current) {
                          current.total += parseFloat(op.amount.value)
                        } else {
                          feesByAccount.set(op.accountAlias, {
                            total: parseFloat(op.amount.value),
                            isDeductible: rule.isDeductibleFrom
                          })
                        }
                      }
                    })

                    let deductibleFees = 0
                    let nonDeductibleFees = 0

                    feesByAccount.forEach((fee) => {
                      if (fee.isDeductible) {
                        deductibleFees += fee.total
                      } else {
                        nonDeductibleFees += fee.total
                      }
                    })

                    const sourcePays = Number(values.value) + nonDeductibleFees
                    const destinationReceives =
                      Number(values.value) - deductibleFees

                    return (
                      <>
                        <TransactionReceiptItem
                          label={intl.formatMessage({
                            id: 'fees.sourcePays',
                            defaultMessage: 'Source pays'
                          })}
                          value={
                            <span className="font-medium">
                              {values.asset}{' '}
                              {intl.formatNumber(sourcePays, {
                                roundingPriority: 'morePrecision'
                              })}
                            </span>
                          }
                        />

                        <TransactionReceiptItem
                          label={intl.formatMessage({
                            id: 'fees.destinationReceives',
                            defaultMessage: 'Destination receives'
                          })}
                          value={
                            <span className="font-medium text-green-600">
                              {values.asset}{' '}
                              {intl.formatNumber(destinationReceives, {
                                roundingPriority: 'morePrecision'
                              })}
                            </span>
                          }
                        />

                        {/* Show explanation for mixed fees */}
                        {deductibleFees > 0 && nonDeductibleFees > 0 && (
                          <TransactionReceiptItem
                            label=""
                            value={
                              <span className="text-xs text-gray-500 italic">
                                {intl.formatMessage(
                                  {
                                    id: 'entity.transactions.mixedFees.explanation',
                                    defaultMessage:
                                      'Mixed fees: {currency} {deductible} deducted from recipient, {currency} {nonDeductible} charged to sender'
                                  },
                                  {
                                    currency: values.asset,
                                    deductible: intl.formatNumber(
                                      deductibleFees,
                                      { roundingPriority: 'morePrecision' }
                                    ),
                                    nonDeductible: intl.formatNumber(
                                      nonDeductibleFees,
                                      { roundingPriority: 'morePrecision' }
                                    )
                                  }
                                )}
                              </span>
                            }
                          />
                        )}
                      </>
                    )
                  })()}
                </>
              )}
          </TransactionReceipt>

          <TransactionReceiptTicket />
        </div>
      </div>

      <PageFooter open>
        <PageFooterSection>
          <Button variant="outline" onClick={handleBack}>
            {intl.formatMessage({
              id: 'common.back',
              defaultMessage: 'Back'
            })}
          </Button>
        </PageFooterSection>
        <PageFooterSection>
          <LoadingButton
            variant="plain"
            loading={loading && sendAnother}
            disabled={loading && !sendAnother}
            onClick={handleSubmitAnother}
          >
            {intl.formatMessage({
              id: 'transactions.create.another.button',
              defaultMessage: 'Send and Create another'
            })}
          </LoadingButton>
          <LoadingButton
            icon={<SendHorizonal />}
            iconPlacement="end"
            loading={loading && !sendAnother}
            disabled={loading && sendAnother}
            onClick={handleSubmit}
          >
            {intl.formatMessage({
              id: 'transactions.create.button',
              defaultMessage: 'Send Transaction'
            })}
          </LoadingButton>
        </PageFooterSection>
      </PageFooter>
    </div>
  )
}
