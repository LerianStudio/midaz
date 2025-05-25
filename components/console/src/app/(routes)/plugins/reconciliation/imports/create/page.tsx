'use client'

import { useState, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import {
  Upload,
  FileText,
  AlertCircle,
  CheckCircle,
  ArrowLeft,
  Download,
  Eye,
  Settings
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

export default function CreateImportPage() {
  const router = useRouter()
  const [currentStep, setCurrentStep] = useState(1)
  const [uploadedFile, setUploadedFile] = useState<File | null>(null)
  const [dragActive, setDragActive] = useState(false)
  const [validationResults, setValidationResults] = useState<any[]>([])
  const [previewData, setPreviewData] = useState<any[]>([])
  const [isValidating, setIsValidating] = useState(false)
  const [isUploading, setIsUploading] = useState(false)

  // Configuration state
  const [config, setConfig] = useState({
    fileType: 'csv',
    delimiter: ',',
    encoding: 'utf-8',
    hasHeaders: true,
    skipRows: 0,
    strictMode: true,
    allowPartialImport: false,
    maxErrorThreshold: 5
  })

  // Field mapping state
  const [fieldMappings, setFieldMappings] = useState([
    {
      sourceField: 'date',
      targetField: 'transaction_date',
      required: true,
      dataType: 'date'
    },
    {
      sourceField: 'amount',
      targetField: 'amount',
      required: true,
      dataType: 'number'
    },
    {
      sourceField: 'description',
      targetField: 'description',
      required: false,
      dataType: 'string'
    },
    {
      sourceField: 'reference',
      targetField: 'reference_number',
      required: false,
      dataType: 'string'
    }
  ])

  const steps = [
    {
      id: 1,
      name: 'Upload File',
      description: 'Select and upload transaction file'
    },
    { id: 2, name: 'Configure', description: 'Configure import settings' },
    {
      id: 3,
      name: 'Map Fields',
      description: 'Map file fields to system fields'
    },
    { id: 4, name: 'Validate', description: 'Validate data and preview' },
    { id: 5, name: 'Import', description: 'Start the import process' }
  ]

  const handleDrag = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true)
    } else if (e.type === 'dragleave') {
      setDragActive(false)
    }
  }, [])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setDragActive(false)

    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      handleFileSelect(e.dataTransfer.files[0])
    }
  }, [])

  const handleFileSelect = (file: File) => {
    setUploadedFile(file)
    // Auto-detect file type
    const extension = file.name.split('.').pop()?.toLowerCase()
    if (extension === 'csv') {
      setConfig((prev) => ({ ...prev, fileType: 'csv' }))
    } else if (extension === 'json') {
      setConfig((prev) => ({ ...prev, fileType: 'json' }))
    } else if (extension === 'xlsx') {
      setConfig((prev) => ({ ...prev, fileType: 'xlsx' }))
    }
  }

  const validateFile = async () => {
    if (!uploadedFile) return

    setIsValidating(true)

    // Mock validation - replace with real API call
    setTimeout(() => {
      const mockValidation = [
        {
          field: 'date',
          isValid: true,
          errors: [],
          warnings: [],
          statistics: { totalValues: 2500, nullValues: 0, validFormat: 2500 }
        },
        {
          field: 'amount',
          isValid: true,
          errors: [],
          warnings: ['3 values with unusual decimal places'],
          statistics: { totalValues: 2500, nullValues: 5, validFormat: 2495 }
        },
        {
          field: 'description',
          isValid: true,
          errors: [],
          warnings: [],
          statistics: { totalValues: 2500, nullValues: 120, validFormat: 2500 }
        },
        {
          field: 'reference',
          isValid: false,
          errors: ['45 duplicate reference numbers found'],
          warnings: [],
          statistics: { totalValues: 2500, nullValues: 200, validFormat: 2255 }
        }
      ]

      const mockPreview = [
        {
          date: '2024-12-01',
          amount: '1250.00',
          description: 'Payment received',
          reference: 'REF001'
        },
        {
          date: '2024-12-01',
          amount: '-75.50',
          description: 'Processing fee',
          reference: 'REF002'
        },
        {
          date: '2024-12-02',
          amount: '2847.82',
          description: 'Wire transfer',
          reference: 'REF003'
        },
        {
          date: '2024-12-02',
          amount: '-12.00',
          description: 'Monthly fee',
          reference: 'REF004'
        },
        {
          date: '2024-12-03',
          amount: '567.25',
          description: 'ACH deposit',
          reference: 'REF005'
        }
      ]

      setValidationResults(mockValidation)
      setPreviewData(mockPreview)
      setIsValidating(false)
    }, 2000)
  }

  const startImport = async () => {
    setIsUploading(true)

    // Mock import start - replace with real API call
    setTimeout(() => {
      setIsUploading(false)
      router.push('/plugins/reconciliation/imports')
    }, 1500)
  }

  const formatFileSize = (bytes: number) => {
    const sizes = ['Bytes', 'KB', 'MB', 'GB']
    if (bytes === 0) return '0 Bytes'
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    return Math.round((bytes / Math.pow(1024, i)) * 100) / 100 + ' ' + sizes[i]
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Button variant="outline" size="sm" onClick={() => router.back()}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h2 className="text-2xl font-bold tracking-tight">
            Import Transaction File
          </h2>
          <p className="text-muted-foreground">
            Upload and configure transaction data for reconciliation
          </p>
        </div>
      </div>

      {/* Progress Steps */}
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center justify-between">
            {steps.map((step, index) => (
              <div key={step.id} className="flex items-center">
                <div
                  className={`flex h-8 w-8 items-center justify-center rounded-full border-2 text-sm font-medium ${
                    currentStep >= step.id
                      ? 'border-primary bg-primary text-primary-foreground'
                      : 'border-muted-foreground text-muted-foreground'
                  }`}
                >
                  {currentStep > step.id ? (
                    <CheckCircle className="h-4 w-4" />
                  ) : (
                    step.id
                  )}
                </div>
                <div className="ml-3 hidden sm:block">
                  <p
                    className={`text-sm font-medium ${currentStep >= step.id ? 'text-foreground' : 'text-muted-foreground'}`}
                  >
                    {step.name}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {step.description}
                  </p>
                </div>
                {index < steps.length - 1 && (
                  <div
                    className={`mx-4 h-px w-12 ${currentStep > step.id ? 'bg-primary' : 'bg-muted'}`}
                  />
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Step Content */}
      {currentStep === 1 && (
        <Card>
          <CardHeader>
            <CardTitle>Upload Transaction File</CardTitle>
            <CardDescription>
              Select a CSV, JSON, or Excel file containing transaction data
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div
              className={`rounded-lg border-2 border-dashed p-8 text-center transition-colors ${dragActive ? 'border-primary bg-primary/5' : 'border-muted-foreground/25'} ${uploadedFile ? 'border-green-500 bg-green-50 dark:bg-green-950/20' : ''} `}
              onDragEnter={handleDrag}
              onDragLeave={handleDrag}
              onDragOver={handleDrag}
              onDrop={handleDrop}
            >
              {uploadedFile ? (
                <div className="space-y-4">
                  <CheckCircle className="mx-auto h-12 w-12 text-green-600" />
                  <div>
                    <h3 className="text-lg font-medium">{uploadedFile.name}</h3>
                    <p className="text-muted-foreground">
                      {formatFileSize(uploadedFile.size)} •{' '}
                      {uploadedFile.type || 'Unknown type'}
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    onClick={() => setUploadedFile(null)}
                  >
                    Choose Different File
                  </Button>
                </div>
              ) : (
                <div className="space-y-4">
                  <Upload className="mx-auto h-12 w-12 text-muted-foreground" />
                  <div>
                    <h3 className="text-lg font-medium">Drop your file here</h3>
                    <p className="text-muted-foreground">
                      or click to browse files
                    </p>
                  </div>
                  <Input
                    type="file"
                    accept=".csv,.json,.xlsx"
                    onChange={(e) =>
                      e.target.files?.[0] && handleFileSelect(e.target.files[0])
                    }
                    className="hidden"
                    id="file-upload"
                  />
                  <Label htmlFor="file-upload">
                    <Button variant="outline" className="gap-2" asChild>
                      <span>
                        <FileText className="h-4 w-4" />
                        Browse Files
                      </span>
                    </Button>
                  </Label>
                </div>
              )}
            </div>

            <div className="flex justify-between">
              <div></div>
              <Button
                onClick={() => setCurrentStep(2)}
                disabled={!uploadedFile}
              >
                Next: Configure
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {currentStep === 2 && (
        <Card>
          <CardHeader>
            <CardTitle>Configure Import Settings</CardTitle>
            <CardDescription>
              Set up file parsing and validation options
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="grid gap-6 md:grid-cols-2">
              <div className="space-y-4">
                <div>
                  <Label htmlFor="fileType">File Type</Label>
                  <Select
                    value={config.fileType}
                    onValueChange={(value) =>
                      setConfig((prev) => ({ ...prev, fileType: value }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="csv">CSV</SelectItem>
                      <SelectItem value="json">JSON</SelectItem>
                      <SelectItem value="xlsx">Excel</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {config.fileType === 'csv' && (
                  <>
                    <div>
                      <Label htmlFor="delimiter">Delimiter</Label>
                      <Select
                        value={config.delimiter}
                        onValueChange={(value) =>
                          setConfig((prev) => ({ ...prev, delimiter: value }))
                        }
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value=",">Comma (,)</SelectItem>
                          <SelectItem value=";">Semicolon (;)</SelectItem>
                          <SelectItem value="\t">Tab</SelectItem>
                          <SelectItem value="|">Pipe (|)</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>

                    <div>
                      <Label htmlFor="encoding">Encoding</Label>
                      <Select
                        value={config.encoding}
                        onValueChange={(value) =>
                          setConfig((prev) => ({ ...prev, encoding: value }))
                        }
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="utf-8">UTF-8</SelectItem>
                          <SelectItem value="iso-8859-1">ISO-8859-1</SelectItem>
                          <SelectItem value="windows-1252">
                            Windows-1252
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </>
                )}

                <div>
                  <Label htmlFor="skipRows">Skip Rows</Label>
                  <Input
                    id="skipRows"
                    type="number"
                    min="0"
                    value={config.skipRows}
                    onChange={(e) =>
                      setConfig((prev) => ({
                        ...prev,
                        skipRows: parseInt(e.target.value) || 0
                      }))
                    }
                  />
                </div>
              </div>

              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <Label htmlFor="hasHeaders">File has headers</Label>
                  <Switch
                    id="hasHeaders"
                    checked={config.hasHeaders}
                    onCheckedChange={(checked) =>
                      setConfig((prev) => ({ ...prev, hasHeaders: checked }))
                    }
                  />
                </div>

                <div className="flex items-center justify-between">
                  <Label htmlFor="strictMode">Strict validation mode</Label>
                  <Switch
                    id="strictMode"
                    checked={config.strictMode}
                    onCheckedChange={(checked) =>
                      setConfig((prev) => ({ ...prev, strictMode: checked }))
                    }
                  />
                </div>

                <div className="flex items-center justify-between">
                  <Label htmlFor="allowPartialImport">
                    Allow partial import
                  </Label>
                  <Switch
                    id="allowPartialImport"
                    checked={config.allowPartialImport}
                    onCheckedChange={(checked) =>
                      setConfig((prev) => ({
                        ...prev,
                        allowPartialImport: checked
                      }))
                    }
                  />
                </div>

                <div>
                  <Label htmlFor="maxErrorThreshold">
                    Max Error Threshold (%)
                  </Label>
                  <Input
                    id="maxErrorThreshold"
                    type="number"
                    min="0"
                    max="100"
                    value={config.maxErrorThreshold}
                    onChange={(e) =>
                      setConfig((prev) => ({
                        ...prev,
                        maxErrorThreshold: parseInt(e.target.value) || 0
                      }))
                    }
                  />
                </div>
              </div>
            </div>

            <div className="flex justify-between">
              <Button variant="outline" onClick={() => setCurrentStep(1)}>
                Back
              </Button>
              <Button onClick={() => setCurrentStep(3)}>
                Next: Map Fields
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {currentStep === 3 && (
        <Card>
          <CardHeader>
            <CardTitle>Map Data Fields</CardTitle>
            <CardDescription>Map file columns to system fields</CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="space-y-4">
              {fieldMappings.map((mapping, index) => (
                <div
                  key={index}
                  className="grid gap-4 rounded-lg border p-4 md:grid-cols-4"
                >
                  <div>
                    <Label>Source Field</Label>
                    <Input
                      value={mapping.sourceField}
                      onChange={(e) => {
                        const newMappings = [...fieldMappings]
                        newMappings[index].sourceField = e.target.value
                        setFieldMappings(newMappings)
                      }}
                    />
                  </div>
                  <div>
                    <Label>Target Field</Label>
                    <Select
                      value={mapping.targetField}
                      onValueChange={(value) => {
                        const newMappings = [...fieldMappings]
                        newMappings[index].targetField = value
                        setFieldMappings(newMappings)
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="transaction_date">
                          Transaction Date
                        </SelectItem>
                        <SelectItem value="amount">Amount</SelectItem>
                        <SelectItem value="description">Description</SelectItem>
                        <SelectItem value="reference_number">
                          Reference Number
                        </SelectItem>
                        <SelectItem value="account_number">
                          Account Number
                        </SelectItem>
                        <SelectItem value="account_name">
                          Account Name
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div>
                    <Label>Data Type</Label>
                    <Select
                      value={mapping.dataType}
                      onValueChange={(value) => {
                        const newMappings = [...fieldMappings]
                        newMappings[index].dataType = value as any
                        setFieldMappings(newMappings)
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="string">String</SelectItem>
                        <SelectItem value="number">Number</SelectItem>
                        <SelectItem value="date">Date</SelectItem>
                        <SelectItem value="boolean">Boolean</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="flex items-center justify-center">
                    <Badge variant={mapping.required ? 'default' : 'secondary'}>
                      {mapping.required ? 'Required' : 'Optional'}
                    </Badge>
                  </div>
                </div>
              ))}
            </div>

            <Button
              variant="outline"
              onClick={() =>
                setFieldMappings([
                  ...fieldMappings,
                  {
                    sourceField: '',
                    targetField: '',
                    required: false,
                    dataType: 'string'
                  }
                ])
              }
            >
              Add Field Mapping
            </Button>

            <div className="flex justify-between">
              <Button variant="outline" onClick={() => setCurrentStep(2)}>
                Back
              </Button>
              <Button onClick={() => setCurrentStep(4)}>Next: Validate</Button>
            </div>
          </CardContent>
        </Card>
      )}

      {currentStep === 4 && (
        <Card>
          <CardHeader>
            <CardTitle>Validate and Preview</CardTitle>
            <CardDescription>
              Review validation results and preview data before importing
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="flex gap-4">
              <Button
                onClick={validateFile}
                disabled={isValidating}
                className="gap-2"
              >
                {isValidating ? (
                  <Settings className="h-4 w-4 animate-spin" />
                ) : (
                  <CheckCircle className="h-4 w-4" />
                )}
                {isValidating ? 'Validating...' : 'Validate Data'}
              </Button>
              {validationResults.length > 0 && (
                <Badge variant="outline" className="gap-2">
                  <CheckCircle className="h-3 w-3" />
                  Validation Complete
                </Badge>
              )}
            </div>

            {isValidating && (
              <div className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span>Validating data...</span>
                  <span>Processing</span>
                </div>
                <Progress value={45} />
              </div>
            )}

            {validationResults.length > 0 && (
              <Tabs defaultValue="results" className="space-y-4">
                <TabsList>
                  <TabsTrigger value="results">Validation Results</TabsTrigger>
                  <TabsTrigger value="preview">Data Preview</TabsTrigger>
                </TabsList>

                <TabsContent value="results" className="space-y-4">
                  {validationResults.map((result, index) => (
                    <div key={index} className="rounded-lg border p-4">
                      <div className="mb-2 flex items-center gap-2">
                        <h4 className="font-medium">{result.field}</h4>
                        {result.isValid ? (
                          <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">
                            Valid
                          </Badge>
                        ) : (
                          <Badge variant="destructive">Invalid</Badge>
                        )}
                      </div>
                      {result.errors.length > 0 && (
                        <div className="space-y-1">
                          {result.errors.map(
                            (error: string, errorIndex: number) => (
                              <div
                                key={errorIndex}
                                className="flex items-center gap-2 text-sm text-red-600"
                              >
                                <AlertCircle className="h-4 w-4" />
                                {error}
                              </div>
                            )
                          )}
                        </div>
                      )}
                      {result.warnings.length > 0 && (
                        <div className="space-y-1">
                          {result.warnings.map(
                            (warning: string, warningIndex: number) => (
                              <div
                                key={warningIndex}
                                className="flex items-center gap-2 text-sm text-yellow-600"
                              >
                                <AlertCircle className="h-4 w-4" />
                                {warning}
                              </div>
                            )
                          )}
                        </div>
                      )}
                      {result.statistics && (
                        <div className="mt-2 grid grid-cols-2 gap-4 text-sm text-muted-foreground md:grid-cols-4">
                          <div>Total: {result.statistics.totalValues}</div>
                          <div>Null: {result.statistics.nullValues}</div>
                          <div>Valid: {result.statistics.validFormat}</div>
                          <div>
                            Invalid:{' '}
                            {result.statistics.totalValues -
                              result.statistics.validFormat}
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </TabsContent>

                <TabsContent value="preview" className="space-y-4">
                  <div className="overflow-hidden rounded-lg border">
                    <div className="overflow-x-auto">
                      <table className="w-full">
                        <thead className="bg-muted">
                          <tr>
                            {Object.keys(previewData[0] || {}).map((header) => (
                              <th
                                key={header}
                                className="px-4 py-2 text-left text-sm font-medium"
                              >
                                {header}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {previewData.map((row, index) => (
                            <tr key={index} className="border-t">
                              {Object.values(row).map(
                                (value: any, cellIndex) => (
                                  <td
                                    key={cellIndex}
                                    className="px-4 py-2 text-sm"
                                  >
                                    {value}
                                  </td>
                                )
                              )}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    Showing first 5 rows of {uploadedFile?.name}
                  </p>
                </TabsContent>
              </Tabs>
            )}

            <div className="flex justify-between">
              <Button variant="outline" onClick={() => setCurrentStep(3)}>
                Back
              </Button>
              <Button
                onClick={() => setCurrentStep(5)}
                disabled={
                  validationResults.length === 0 ||
                  validationResults.some((r) => !r.isValid)
                }
              >
                Next: Import
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {currentStep === 5 && (
        <Card>
          <CardHeader>
            <CardTitle>Start Import</CardTitle>
            <CardDescription>
              Review settings and start the import process
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="space-y-4">
              <div className="rounded-lg bg-muted p-4">
                <h4 className="mb-2 font-medium">Import Summary</h4>
                <div className="grid gap-2 text-sm">
                  <div className="flex justify-between">
                    <span>File:</span>
                    <span>{uploadedFile?.name}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>Size:</span>
                    <span>
                      {uploadedFile ? formatFileSize(uploadedFile.size) : 'N/A'}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span>Expected Records:</span>
                    <span>2,500</span>
                  </div>
                  <div className="flex justify-between">
                    <span>Field Mappings:</span>
                    <span>{fieldMappings.length}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>Validation:</span>
                    <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">
                      Passed
                    </Badge>
                  </div>
                </div>
              </div>

              {isUploading && (
                <div className="space-y-2">
                  <div className="flex justify-between text-sm">
                    <span>Importing data...</span>
                    <span>Processing</span>
                  </div>
                  <Progress value={75} />
                </div>
              )}
            </div>

            <div className="flex justify-between">
              <Button variant="outline" onClick={() => setCurrentStep(4)}>
                Back
              </Button>
              <Button
                onClick={startImport}
                disabled={isUploading}
                className="gap-2"
              >
                {isUploading ? (
                  <Settings className="h-4 w-4 animate-spin" />
                ) : (
                  <Upload className="h-4 w-4" />
                )}
                {isUploading ? 'Importing...' : 'Start Import'}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
