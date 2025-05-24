'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { FeeCalculatorForm } from '@/components/fees/calculator/fee-calculator-form'
import { CalculationResult } from '@/components/fees/calculator/calculation-result'
import { CalculationBreakdown } from '@/components/fees/calculator/calculation-breakdown'
import {
  generateMockCalculation,
  getActivePackages,
  generateSampleTransactions
} from '@/components/fees/mock/fee-mock-data'
import {
  FeeCalculationRequest,
  FeeCalculationResponse
} from '@/components/fees/types/fee-types'
import { Calculator, FileText, TrendingUp } from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

export default function CalculatorPage() {
  const intl = useIntl()
  const [calculationResult, setCalculationResult] =
    React.useState<FeeCalculationResponse | null>(null)
  const [isCalculating, setIsCalculating] = React.useState(false)
  const [calculationHistory, setCalculationHistory] = React.useState<
    Array<{
      request: FeeCalculationRequest
      response: FeeCalculationResponse
      timestamp: Date
    }>
  >([])

  const handleCalculate = async (request: FeeCalculationRequest) => {
    setIsCalculating(true)

    try {
      // Simulate API call delay
      await new Promise((resolve) => setTimeout(resolve, 500))

      // Generate mock calculation
      const result = generateMockCalculation(
        request.amount,
        request.packageId || getActivePackages()[0]?.id || '',
        request.from,
        request.to
      )

      setCalculationResult(result)

      // Add to history
      setCalculationHistory((prev) =>
        [
          {
            request,
            response: result,
            timestamp: new Date()
          },
          ...prev
        ].slice(0, 10)
      ) // Keep last 10 calculations
    } catch (error) {
      console.error('Calculation failed:', error)
    } finally {
      setIsCalculating(false)
    }
  }

  const sampleTransactions = generateSampleTransactions()

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <PageHeader.InfoTitle
          title={intl.formatMessage({
            id: 'fees.calculator.title',
            defaultMessage: 'Fee Calculator'
          })}
          subtitle={intl.formatMessage({
            id: 'fees.calculator.subtitle',
            defaultMessage:
              'Test and preview fee calculations for your transactions'
          })}
        />
      </PageHeader.Root>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Calculator Form */}
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Calculator className="h-5 w-5" />
                Calculate Fees
              </CardTitle>
            </CardHeader>
            <CardContent>
              <FeeCalculatorForm
                onCalculate={handleCalculate}
                isCalculating={isCalculating}
              />
            </CardContent>
          </Card>

          {/* Sample Transactions */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileText className="h-5 w-5" />
                Sample Transactions
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="mb-4 text-sm text-muted-foreground">
                Click on a sample to populate the calculator
              </p>
              <div className="space-y-2">
                {sampleTransactions.map((sample, index) => (
                  <button
                    key={index}
                    className="w-full rounded-lg border p-3 text-left transition-colors hover:bg-accent"
                    onClick={() => {
                      // Populate form with sample data
                      const form = document.querySelector(
                        'form'
                      ) as HTMLFormElement
                      if (form) {
                        ;(
                          form.querySelector(
                            'input[name="amount"]'
                          ) as HTMLInputElement
                        ).value = sample.amount.toString()
                        ;(
                          form.querySelector(
                            'input[name="from"]'
                          ) as HTMLInputElement
                        ).value = sample.from
                        ;(
                          form.querySelector(
                            'input[name="to"]'
                          ) as HTMLInputElement
                        ).value = sample.to
                      }
                    }}
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-medium">{sample.description}</span>
                      <span className="text-sm text-muted-foreground">
                        ${sample.amount.toLocaleString()}
                      </span>
                    </div>
                  </button>
                ))}
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Results */}
        <div className="space-y-6">
          {calculationResult ? (
            <>
              <CalculationResult result={calculationResult} />
              <CalculationBreakdown result={calculationResult} />
            </>
          ) : (
            <Card className="flex h-full items-center justify-center">
              <CardContent className="py-12 text-center">
                <Calculator className="mx-auto mb-4 h-12 w-12 text-muted-foreground/50" />
                <p className="text-muted-foreground">
                  Enter transaction details to calculate fees
                </p>
              </CardContent>
            </Card>
          )}
        </div>
      </div>

      {/* Calculation History */}
      {calculationHistory.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <TrendingUp className="h-5 w-5" />
              Recent Calculations
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {calculationHistory.map((item, index) => (
                <div
                  key={index}
                  className="flex cursor-pointer items-center justify-between rounded-lg border p-3 hover:bg-accent"
                  onClick={() => setCalculationResult(item.response)}
                >
                  <div>
                    <p className="font-medium">
                      ${item.request.amount.toLocaleString()} transaction
                    </p>
                    <p className="text-sm text-muted-foreground">
                      {item.request.from} → {item.request.to}
                    </p>
                  </div>
                  <div className="text-right">
                    <Badge variant="secondary">
                      ${item.response.totalFees.toFixed(2)} fee
                    </Badge>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.timestamp.toLocaleTimeString()}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
