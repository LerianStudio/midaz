'use client'

import React, { useState } from 'react'
import { Button } from '@/components/ui/button'
import { InputField } from '@/components/form/input-field'
import { SelectField } from '@/components/form/select-field'
import { SelectItem } from '@/components/ui/select'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Calculator, RotateCcw } from 'lucide-react'
import { useForm, FormProvider } from 'react-hook-form'
import { Separator } from '@/components/ui/separator'
import { PageHeader } from '@/components/page-header'
import { Breadcrumb } from '@/components/breadcrumb'

// Types for our simulation
type SimulationType = 'cash-in' | 'cash-out'
type AccountType = 'checking' | 'savings' | 'investment'
type TransactionType = 'pix' | 'ted' | 'doc' | 'boleto'

interface SimulationResult {
  originalValue: number
  fee: number
  finalValue: number
  estimatedTime: string
}

// Main component
export default function Page() {
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<SimulationResult | null>(null)

  const form = useForm({
    defaultValues: {
      simulationType: 'cash-in' as SimulationType,
      value: '',
      accountType: 'checking' as AccountType,
      transactionType: 'pix' as TransactionType
    }
  })

  const simulationType = form.watch('simulationType')
  const value = form.watch('value')
  const accountType = form.watch('accountType')
  const transactionType = form.watch('transactionType')

  const handleSimulation = (data: any) => {
    setLoading(true)

    // Simulate API call
    setTimeout(() => {
      // Calculate fee based on simulation parameters
      let fee = 0
      const numericValue = parseFloat(data.value)

      if (data.simulationType === 'cash-in') {
        if (data.transactionType === 'pix') {
          fee = 0 // PIX is free for cash-in
        } else if (data.transactionType === 'ted') {
          fee = numericValue * 0.01 // 1% for TED
        } else if (data.transactionType === 'doc') {
          fee = numericValue * 0.015 // 1.5% for DOC
        } else if (data.transactionType === 'boleto') {
          fee = 2.5 // Fixed fee for boleto
        }
      } else {
        // cash-out
        if (data.transactionType === 'pix') {
          fee = numericValue * 0.005 // 0.5% for PIX cash-out
        } else if (data.transactionType === 'ted') {
          fee = numericValue * 0.015 // 1.5% for TED cash-out
        } else if (data.transactionType === 'doc') {
          fee = numericValue * 0.02 // 2% for DOC cash-out
        } else if (data.transactionType === 'boleto') {
          fee = 5 // Higher fixed fee for boleto cash-out
        }
      }

      // Adjust fee based on account type
      if (data.accountType === 'investment') {
        fee = fee * 0.8 // 20% discount for investment accounts
      }

      // Calculate final value
      const finalValue =
        data.simulationType === 'cash-in'
          ? numericValue - fee
          : numericValue + fee

      // Estimate time
      let estimatedTime = ''
      if (data.transactionType === 'pix') {
        estimatedTime = 'Imediato'
      } else if (data.transactionType === 'ted') {
        estimatedTime = 'Até 30 minutos'
      } else if (data.transactionType === 'doc') {
        estimatedTime = 'D+1 (próximo dia útil)'
      } else if (data.transactionType === 'boleto') {
        estimatedTime = 'D+1 a D+3 (dias úteis)'
      }

      setResult({
        originalValue: numericValue,
        fee,
        finalValue,
        estimatedTime
      })

      setLoading(false)
    }, 800)
  }

  const handleReset = () => {
    form.reset()
    setResult(null)
  }

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat('pt-BR', {
      style: 'currency',
      currency: 'BRL'
    }).format(value)
  }

  return (
    <>
      <PageHeader.Root>
        <Breadcrumb
          paths={[
            { name: 'Home', href: '/' },
            { name: 'Simulação de Transação', href: '/transaction-simulation' }
          ]}
        />
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title="Simulação de Transação"
            subtitle="Simule operações de cash-in e cash-out para diferentes tipos de transação."
          />
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <div className="container mx-auto p-6">
        <Card>
          <CardHeader>
            <CardTitle>Simulação de Cash-in/Cash-out via Transação</CardTitle>
          </CardHeader>
          <CardContent>
            <FormProvider {...form}>
              <form
                className="space-y-6"
                onSubmit={form.handleSubmit(handleSimulation)}
              >
                <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                  <div>
                    <SelectField
                      name="simulationType"
                      control={form.control}
                      label="Tipo de Simulação"
                      required
                    >
                      <SelectItem value="cash-in">Cash-in (Entrada)</SelectItem>
                      <SelectItem value="cash-out">Cash-out (Saída)</SelectItem>
                    </SelectField>
                  </div>

                  <div>
                    <InputField
                      name="value"
                      control={form.control}
                      type="number"
                      label="Valor"
                      placeholder="0,00"
                      required
                    />
                  </div>

                  <div>
                    <SelectField
                      name="accountType"
                      control={form.control}
                      label="Tipo de Conta"
                      required
                    >
                      <SelectItem value="checking">Conta Corrente</SelectItem>
                      <SelectItem value="savings">Conta Poupança</SelectItem>
                      <SelectItem value="investment">
                        Conta Investimento
                      </SelectItem>
                    </SelectField>
                  </div>

                  <div>
                    <SelectField
                      name="transactionType"
                      control={form.control}
                      label="Tipo de Transação"
                      required
                    >
                      <SelectItem value="pix">PIX</SelectItem>
                      <SelectItem value="ted">TED</SelectItem>
                      <SelectItem value="doc">DOC</SelectItem>
                      <SelectItem value="boleto">Boleto</SelectItem>
                    </SelectField>
                  </div>
                </div>

                <div className="flex justify-end space-x-4">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleReset}
                    icon={<RotateCcw className="h-4 w-4" />}
                    disabled={loading}
                  >
                    Resetar
                  </Button>
                  <Button
                    type="submit"
                    icon={<Calculator className="h-4 w-4" />}
                    disabled={loading || !value}
                  >
                    Simular
                  </Button>
                </div>
              </form>
            </FormProvider>

            {result && (
              <>
                <Separator className="my-6" />

                <div className="rounded-lg border border-shadcn-200 p-6">
                  <h3 className="mb-4 text-lg font-medium">
                    Resultado da Simulação
                  </h3>

                  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                    <div>
                      <p className="text-sm text-shadcn-500">
                        Tipo de Operação
                      </p>
                      <p className="font-medium">
                        {simulationType === 'cash-in'
                          ? 'Cash-in (Entrada)'
                          : 'Cash-out (Saída)'}
                      </p>
                    </div>

                    <div>
                      <p className="text-sm text-shadcn-500">Valor Original</p>
                      <p className="font-medium">
                        {formatCurrency(result.originalValue)}
                      </p>
                    </div>

                    <div>
                      <p className="text-sm text-shadcn-500">Taxa</p>
                      <p className="font-medium">
                        {formatCurrency(result.fee)}
                      </p>
                    </div>

                    <div>
                      <p className="text-sm text-shadcn-500">Valor Final</p>
                      <p
                        className={`text-lg font-medium ${simulationType === 'cash-in' ? 'text-green-600' : 'text-red-600'}`}
                      >
                        {formatCurrency(result.finalValue)}
                      </p>
                    </div>

                    <div>
                      <p className="text-sm text-shadcn-500">Tipo de Conta</p>
                      <p className="font-medium">
                        {accountType === 'checking'
                          ? 'Conta Corrente'
                          : accountType === 'savings'
                            ? 'Conta Poupança'
                            : 'Conta Investimento'}
                      </p>
                    </div>

                    <div>
                      <p className="text-sm text-shadcn-500">Tempo Estimado</p>
                      <p className="font-medium">{result.estimatedTime}</p>
                    </div>
                  </div>

                  <div className="bg-shadcn-50 mt-6 rounded-md p-4">
                    <p className="text-sm text-shadcn-500">
                      <strong>Nota:</strong> Os valores reais podem variar de
                      acordo com as políticas da instituição financeira e o
                      momento da transação.
                    </p>
                  </div>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  )
}
