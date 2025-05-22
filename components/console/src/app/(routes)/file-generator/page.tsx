'use client'

import React, { useState } from 'react'
import { useForm, FormProvider } from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Check, Download, FileText, X } from 'lucide-react'
import { Checkbox } from '@/components/ui/checkbox'
import { Progress } from '@/components/ui/progress'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { PageHeader } from '@/components/page-header'
import { Breadcrumb } from '@/components/breadcrumb'
import { SelectField } from '@/components/form/select-field'
import { SelectItem } from '@/components/ui/select'
import { InputField } from '@/components/form/input-field'

// Types for our file generation
interface FileType {
  id: string
  name: string
  description: string
  selected: boolean
}

interface GeneratedFile {
  id: string
  name: string
  status: 'success' | 'error' | 'generating'
  downloadUrl?: string
  error?: string
}

// Mock file types
const fileTypes: FileType[] = [
  {
    id: 'remessa',
    name: 'Arquivo de Remessa',
    description: 'Arquivo CNAB 240 para envio de cobranças',
    selected: false
  },
  {
    id: 'retorno',
    name: 'Arquivo de Retorno',
    description: 'Arquivo CNAB 240 com retornos de cobranças',
    selected: false
  },
  {
    id: 'conciliacao',
    name: 'Arquivo de Conciliação',
    description: 'Arquivo CSV para conciliação bancária',
    selected: false
  },
  {
    id: 'extrato',
    name: 'Extrato Bancário',
    description: 'Arquivo PDF com extrato bancário',
    selected: false
  },
  {
    id: 'relatorio',
    name: 'Relatório de Transações',
    description: 'Arquivo Excel com relatório de transações',
    selected: false
  }
]

// Main component
export default function Page() {
  const [selectedTypes, setSelectedTypes] = useState<FileType[]>([])
  const [generatingFiles, setGeneratingFiles] = useState(false)
  const [progress, setProgress] = useState(0)
  const [generatedFiles, setGeneratedFiles] = useState<GeneratedFile[]>([])
  const [showResults, setShowResults] = useState(false)

  const form = useForm({
    defaultValues: {
      dateRange: 'last30days',
      customStartDate: '',
      customEndDate: '',
      format: 'original'
    }
  })

  const dateRange = form.watch('dateRange')

  const toggleFileType = (id: string) => {
    const updatedTypes = fileTypes.map((type) => {
      if (type.id === id) {
        return { ...type, selected: !type.selected }
      }
      return type
    })

    const selected = updatedTypes.filter((type) => type.selected)
    setSelectedTypes(selected)
  }

  const generateFiles = (data: any) => {
    if (selectedTypes.length === 0) {
      return
    }

    setGeneratingFiles(true)
    setProgress(0)
    setGeneratedFiles([])
    setShowResults(false)

    // Initialize generating files
    const initialFiles = selectedTypes.map((type) => ({
      id: type.id,
      name: type.name,
      status: 'generating' as const
    }))

    setGeneratedFiles(initialFiles)

    // Simulate file generation with progress
    const totalSteps = 10
    let currentStep = 0

    const interval = setInterval(() => {
      currentStep++
      setProgress(Math.round((currentStep / totalSteps) * 100))

      if (currentStep === totalSteps) {
        clearInterval(interval)

        // Simulate completed files with some random success/errors
        const completedFiles = selectedTypes.map((type) => {
          const isSuccess = Math.random() > 0.2 // 80% success rate

          return {
            id: type.id,
            name: type.name,
            status: isSuccess ? ('success' as const) : ('error' as const),
            downloadUrl: isSuccess ? `#download-${type.id}` : undefined,
            error: isSuccess
              ? undefined
              : 'Erro ao gerar arquivo. Tente novamente.'
          }
        })

        setGeneratedFiles(completedFiles)
        setGeneratingFiles(false)
        setShowResults(true)
      }
    }, 500)
  }

  const resetForm = () => {
    // Reset selected file types
    fileTypes.forEach((type) => {
      type.selected = false
    })

    setSelectedTypes([])
    setGeneratedFiles([])
    setShowResults(false)
    form.reset()
  }

  return (
    <>
      <PageHeader.Root>
        <Breadcrumb
          paths={[
            { name: 'Home', href: '/' },
            { name: 'Gerador de Arquivos', href: '/file-generator' }
          ]}
        />
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title="Gerador de Arquivos"
            subtitle="Gere e baixe arquivos financeiros para integração com outros sistemas."
          />
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <div className="container mx-auto p-6">
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Selecione os Tipos de Arquivo</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
              {fileTypes.map((type) => (
                <div
                  key={type.id}
                  className={`cursor-pointer rounded-lg border p-4 transition-colors ${
                    type.selected
                      ? 'border-primary bg-primary/5'
                      : 'border-shadcn-200'
                  }`}
                  onClick={() => toggleFileType(type.id)}
                >
                  <div className="flex items-start space-x-3">
                    <Checkbox
                      checked={type.selected}
                      onCheckedChange={() => toggleFileType(type.id)}
                      className="mt-1"
                    />
                    <div>
                      <h3 className="font-medium">{type.name}</h3>
                      <p className="text-sm text-shadcn-500">
                        {type.description}
                      </p>
                    </div>
                  </div>
                </div>
              ))}
            </div>

            {selectedTypes.length > 0 && (
              <div className="mt-4 flex items-center text-sm text-shadcn-500">
                <span>{selectedTypes.length} tipo(s) selecionado(s)</span>
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Configurações</CardTitle>
          </CardHeader>
          <CardContent>
            <FormProvider {...form}>
              <form onSubmit={form.handleSubmit(generateFiles)}>
                <div className="space-y-6">
                  <div>
                    <SelectField
                      name="dateRange"
                      control={form.control}
                      label="Período"
                    >
                      <SelectItem value="today">Hoje</SelectItem>
                      <SelectItem value="yesterday">Ontem</SelectItem>
                      <SelectItem value="last7days">Últimos 7 dias</SelectItem>
                      <SelectItem value="last30days">
                        Últimos 30 dias
                      </SelectItem>
                      <SelectItem value="thisMonth">Este mês</SelectItem>
                      <SelectItem value="lastMonth">Mês passado</SelectItem>
                      <SelectItem value="custom">
                        Período personalizado
                      </SelectItem>
                    </SelectField>
                  </div>

                  {dateRange === 'custom' && (
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                      <InputField
                        name="customStartDate"
                        control={form.control}
                        type="date"
                        label="Data inicial"
                        required
                      />
                      <InputField
                        name="customEndDate"
                        control={form.control}
                        type="date"
                        label="Data final"
                        required
                      />
                    </div>
                  )}

                  <div>
                    <SelectField
                      name="format"
                      control={form.control}
                      label="Formato de saída"
                    >
                      <SelectItem value="original">Formato original</SelectItem>
                      <SelectItem value="csv">CSV</SelectItem>
                      <SelectItem value="excel">Excel</SelectItem>
                      <SelectItem value="pdf">PDF</SelectItem>
                      <SelectItem value="json">JSON</SelectItem>
                    </SelectField>
                  </div>

                  <div className="flex justify-end space-x-4">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={resetForm}
                      disabled={generatingFiles}
                    >
                      Limpar
                    </Button>
                    <Button
                      type="submit"
                      icon={<FileText className="h-4 w-4" />}
                      disabled={generatingFiles || selectedTypes.length === 0}
                    >
                      Gerar Arquivos
                    </Button>
                  </div>
                </div>
              </form>
            </FormProvider>
          </CardContent>
        </Card>

        {generatingFiles && (
          <Card>
            <CardHeader>
              <CardTitle>Gerando Arquivos</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <Progress
                  value={progress}
                  className="h-2"
                  indicatorColor="bg-primary"
                />
                <p className="text-center text-sm text-shadcn-500">
                  {progress}% completo
                </p>

                <div className="space-y-2">
                  {generatedFiles.map((file) => (
                    <div
                      key={file.id}
                      className="flex items-center justify-between border-b pb-2"
                    >
                      <span>{file.name}</span>
                      <Badge variant="outline">Gerando...</Badge>
                    </div>
                  ))}
                </div>
              </div>
            </CardContent>
          </Card>
        )}

        {showResults && (
          <Card>
            <CardHeader>
              <CardTitle>Arquivos Gerados</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {generatedFiles.map((file) => (
                  <div
                    key={file.id}
                    className="flex items-center justify-between border-b pb-3"
                  >
                    <div className="flex items-center space-x-2">
                      {file.status === 'success' ? (
                        <Check className="h-5 w-5 text-green-500" />
                      ) : (
                        <X className="h-5 w-5 text-red-500" />
                      )}
                      <span>{file.name}</span>
                    </div>

                    <div>
                      {file.status === 'success' ? (
                        <Button
                          size="sm"
                          variant="outline"
                          icon={<Download className="h-4 w-4" />}
                          onClick={() =>
                            window.open(file.downloadUrl, '_blank')
                          }
                        >
                          Baixar
                        </Button>
                      ) : (
                        <Badge variant="destructive">Erro</Badge>
                      )}
                    </div>
                  </div>
                ))}

                <Separator />

                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm text-shadcn-500">
                      {
                        generatedFiles.filter((f) => f.status === 'success')
                          .length
                      }{' '}
                      de {generatedFiles.length} arquivos gerados com sucesso
                    </p>
                  </div>

                  <Button variant="outline" onClick={resetForm}>
                    Gerar Novos Arquivos
                  </Button>
                </div>

                {generatedFiles.some((f) => f.status === 'error') && (
                  <Alert variant="destructive" className="mt-4">
                    <AlertDescription>
                      Alguns arquivos não puderam ser gerados. Tente novamente
                      ou entre em contato com o suporte.
                    </AlertDescription>
                  </Alert>
                )}
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </>
  )
}
