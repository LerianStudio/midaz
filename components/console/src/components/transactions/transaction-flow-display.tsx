import { cn } from '@/lib/utils'
import {
  TransactionFlow,
  TransactionDisplayData
} from '@/types/transaction-display.types'
import { useIntl } from 'react-intl'
import { ArrowRight, GitBranch, AlertCircle } from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'

interface TransactionFlowDisplayProps {
  displayData: TransactionDisplayData
  className?: string
  showDetails?: boolean
}

export function TransactionFlowDisplay({
  displayData,
  className,
  showDetails = true
}: TransactionFlowDisplayProps) {
  const _intl = useIntl()

  if (displayData.displayMode === 'simple' && displayData.flows.length === 1) {
    return (
      <SimpleTransactionFlow
        flow={displayData.flows[0]}
        asset={displayData.asset}
        className={className}
        showDetails={showDetails}
      />
    )
  }

  return (
    <ComplexTransactionFlow
      displayData={displayData}
      className={className}
      showDetails={showDetails}
    />
  )
}

interface SimpleTransactionFlowProps {
  flow: TransactionFlow
  asset: string
  className?: string
  showDetails?: boolean
}

function SimpleTransactionFlow({
  flow,
  asset,
  className,
  showDetails
}: SimpleTransactionFlowProps) {
  const intl = useIntl()

  return (
    <Card className={cn('p-6', className)}>
      <div className="flex items-center justify-between">
        {/* Source */}
        <div className="flex-1">
          <div className="text-muted-foreground mb-1 text-sm">
            {intl.formatMessage({
              id: 'transactions.source',
              defaultMessage: 'Source'
            })}
          </div>
          <div className="font-medium">{flow.sourceOperation.accountAlias}</div>
          <div className="text-2xl font-bold text-red-600">
            -{asset} {flow.sourceAmount}
          </div>
        </div>

        {/* Arrow */}
        <div className="mx-6">
          <ArrowRight className="text-muted-foreground h-6 w-6" />
        </div>

        {/* Destination */}
        <div className="flex-1 text-right">
          <div className="text-muted-foreground mb-1 text-sm">
            {intl.formatMessage({
              id: 'transactions.destination',
              defaultMessage: 'Destination'
            })}
          </div>
          <div className="font-medium">
            {flow.destinationOperations[0].accountAlias}
          </div>
          <div className="text-2xl font-bold text-green-600">
            +{asset} {flow.destinationTotalAmount}
          </div>
        </div>
      </div>

      {/* Fees */}
      {flow.feeOperations.length > 0 && showDetails && (
        <>
          <Separator className="my-4" />
          <div className="space-y-2">
            <div className="text-muted-foreground text-sm font-medium">
              {intl.formatMessage({
                id: 'transactions.fees',
                defaultMessage: 'Fees'
              })}
            </div>
            {flow.feeOperations.map((feeOp) => (
              <div
                key={feeOp.operationId}
                className="flex items-center justify-between"
              >
                <div className="flex items-center gap-2">
                  <span className="text-sm">{feeOp.accountAlias}</span>
                  <Badge
                    variant={
                      feeOp.feeType === 'deductible' ? 'secondary' : 'outline'
                    }
                    className="text-xs"
                  >
                    {feeOp.feeType === 'deductible'
                      ? intl.formatMessage({
                          id: 'fees.deductible',
                          defaultMessage: 'Deductible'
                        })
                      : intl.formatMessage({
                          id: 'fees.nonDeductible',
                          defaultMessage: 'Non-deductible'
                        })}
                  </Badge>
                </div>
                <span className="text-sm font-medium text-blue-600">
                  +{asset} {feeOp.amount}
                </span>
              </div>
            ))}
          </div>
        </>
      )}
    </Card>
  )
}

interface ComplexTransactionFlowProps {
  displayData: TransactionDisplayData
  className?: string
  showDetails?: boolean
}

function ComplexTransactionFlow({
  displayData,
  className,
  showDetails
}: ComplexTransactionFlowProps) {
  const intl = useIntl()

  return (
    <div className={cn('space-y-4', className)}>
      {/* Summary Card */}
      <Card className="bg-muted/50 p-6">
        <div className="mb-4 flex items-center gap-2">
          <GitBranch className="text-muted-foreground h-5 w-5" />
          <h3 className="text-lg font-semibold">
            {intl.formatMessage({
              id: 'transactions.multiParty',
              defaultMessage: 'Multi-party Transaction'
            })}
          </h3>
        </div>

        <div className="grid grid-cols-3 gap-4 text-center">
          <div>
            <div className="text-muted-foreground text-sm">
              {intl.formatMessage({
                id: 'transactions.totalSent',
                defaultMessage: 'Total Sent'
              })}
            </div>
            <div className="text-xl font-bold text-red-600">
              -{displayData.asset} {displayData.summary.totalSourceAmount}
            </div>
          </div>
          <div>
            <div className="text-muted-foreground text-sm">
              {intl.formatMessage({
                id: 'transactions.totalReceived',
                defaultMessage: 'Total Received'
              })}
            </div>
            <div className="text-xl font-bold text-green-600">
              +{displayData.asset} {displayData.summary.totalDestinationAmount}
            </div>
          </div>
          <div>
            <div className="text-muted-foreground text-sm">
              {intl.formatMessage({
                id: 'transactions.fees.total',
                defaultMessage: 'Total Fees'
              })}
            </div>
            <div className="text-xl font-bold text-blue-600">
              +{displayData.asset} {displayData.summary.totalFeeAmount}
            </div>
          </div>
        </div>

        {displayData.hasWarnings && (
          <div className="mt-4 rounded-md bg-yellow-50 p-3 dark:bg-yellow-900/20">
            <div className="flex items-start gap-2">
              <AlertCircle className="mt-0.5 h-4 w-4 text-yellow-600 dark:text-yellow-500" />
              <div className="space-y-1">
                {displayData.warnings.map((warning, index) => (
                  <p
                    key={index}
                    className="text-sm text-yellow-800 dark:text-yellow-200"
                  >
                    {warning}
                  </p>
                ))}
              </div>
            </div>
          </div>
        )}
      </Card>

      {/* Individual Flows */}
      {showDetails &&
        displayData.flows.map((flow, index) => (
          <Card key={flow.flowId} className="p-4">
            <div className="text-muted-foreground mb-2 text-sm">
              {intl.formatMessage(
                { id: 'transactions.flow', defaultMessage: 'Flow {number}' },
                { number: index + 1 }
              )}
            </div>

            <div className="grid grid-cols-[1fr,auto,2fr] items-start gap-4">
              {/* Source */}
              <div className="space-y-1">
                <div className="font-medium">
                  {flow.sourceOperation.accountAlias}
                </div>
                <div className="text-lg font-semibold text-red-600">
                  -{displayData.asset} {flow.sourceAmount}
                </div>
              </div>

              {/* Arrow */}
              <div className="flex h-full items-center">
                <ArrowRight className="text-muted-foreground h-5 w-5" />
              </div>

              {/* Destinations and Fees */}
              <div className="space-y-3">
                {/* Main destinations */}
                <div className="space-y-2">
                  {flow.destinationOperations.map((destOp) => (
                    <div
                      key={destOp.operationId}
                      className="flex items-center justify-between"
                    >
                      <span className="font-medium">{destOp.accountAlias}</span>
                      <span className="font-semibold text-green-600">
                        +{displayData.asset} {destOp.amount}
                      </span>
                    </div>
                  ))}
                </div>

                {/* Fees for this flow */}
                {flow.feeOperations.length > 0 && (
                  <div className="space-y-2 border-t pt-2">
                    <div className="text-muted-foreground text-xs">
                      {intl.formatMessage({
                        id: 'transactions.fees',
                        defaultMessage: 'Fees'
                      })}
                    </div>
                    {flow.feeOperations.map((feeOp) => (
                      <div
                        key={feeOp.operationId}
                        className="flex items-center justify-between"
                      >
                        <div className="flex items-center gap-2">
                          <span className="text-sm">{feeOp.accountAlias}</span>
                          <TooltipProvider>
                            <Tooltip>
                              <TooltipTrigger>
                                <Badge
                                  variant={
                                    feeOp.feeType === 'deductible'
                                      ? 'secondary'
                                      : 'outline'
                                  }
                                  className="text-xs"
                                >
                                  {feeOp.feeType === 'deductible' ? 'D' : 'ND'}
                                </Badge>
                              </TooltipTrigger>
                              <TooltipContent>
                                {feeOp.feeType === 'deductible'
                                  ? intl.formatMessage({
                                      id: 'fees.deductible.tooltip',
                                      defaultMessage:
                                        'Deducted from recipient amount'
                                    })
                                  : intl.formatMessage({
                                      id: 'fees.nonDeductible.tooltip',
                                      defaultMessage: 'Added to sender amount'
                                    })}
                              </TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        </div>
                        <span className="text-sm font-medium text-blue-600">
                          +{displayData.asset} {feeOp.amount}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </Card>
        ))}

      {/* Participant Summary */}
      {showDetails && (
        <Card className="p-4">
          <h4 className="mb-3 text-sm font-medium">
            {intl.formatMessage({
              id: 'transactions.participants',
              defaultMessage: 'Transaction Participants'
            })}
          </h4>
          <div className="grid grid-cols-3 gap-4 text-sm">
            <div>
              <div className="text-muted-foreground mb-1">
                {intl.formatMessage(
                  {
                    id: 'transactions.sources',
                    defaultMessage: 'Sources ({count})'
                  },
                  { count: displayData.summary.uniqueSourceAccounts.length }
                )}
              </div>
              <div className="space-y-1">
                {displayData.summary.uniqueSourceAccounts.map((account) => (
                  <div key={account}>{account}</div>
                ))}
              </div>
            </div>
            <div>
              <div className="text-muted-foreground mb-1">
                {intl.formatMessage(
                  {
                    id: 'transactions.destinations',
                    defaultMessage: 'Destinations ({count})'
                  },
                  {
                    count: displayData.summary.uniqueDestinationAccounts.length
                  }
                )}
              </div>
              <div className="space-y-1">
                {displayData.summary.uniqueDestinationAccounts.map(
                  (account) => (
                    <div key={account}>{account}</div>
                  )
                )}
              </div>
            </div>
            <div>
              <div className="text-muted-foreground mb-1">
                {intl.formatMessage(
                  {
                    id: 'transactions.feeRecipients',
                    defaultMessage: 'Fee Recipients ({count})'
                  },
                  { count: displayData.summary.uniqueFeeAccounts.length }
                )}
              </div>
              <div className="space-y-1">
                {displayData.summary.uniqueFeeAccounts.map((account) => (
                  <div key={account}>{account}</div>
                ))}
              </div>
            </div>
          </div>
        </Card>
      )}
    </div>
  )
}
