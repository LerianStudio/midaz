'use client'

import React, { useState, useCallback } from 'react'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger
} from '@/components/ui/accordion'
import { useToast } from '@/hooks/use-toast'
import {
  Send,
  Save,
  Clock,
  FileText,
  Key,
  Plus,
  Trash2,
  Play,
  Download,
  Upload,
  Copy,
  CheckCircle,
  XCircle,
  AlertCircle,
  Code,
  Shield,
  Zap,
  History,
  FolderOpen,
  Settings,
  Database,
  DollarSign
} from 'lucide-react'
import Editor from '@monaco-editor/react'

type HttpMethod =
  | 'GET'
  | 'POST'
  | 'PUT'
  | 'PATCH'
  | 'DELETE'
  | 'HEAD'
  | 'OPTIONS'
type AuthType = 'none' | 'bearer' | 'api-key' | 'basic' | 'oauth2'

interface Header {
  id: string
  key: string
  value: string
  enabled: boolean
}

interface QueryParam {
  id: string
  key: string
  value: string
  enabled: boolean
}

interface TestRequest {
  id: string
  name: string
  url: string
  method: HttpMethod
  headers: Header[]
  queryParams: QueryParam[]
  authType: AuthType
  authConfig: Record<string, string>
  body: string
  validations: ValidationRule[]
}

interface TestResponse {
  status: number
  statusText: string
  headers: Record<string, string>
  body: any
  time: number
  size: number
}

interface TestExecution {
  id: string
  request: TestRequest
  response: TestResponse
  timestamp: Date
  passed: boolean
  validationResults: ValidationResult[]
}

interface ValidationRule {
  id: string
  name: string
  type: 'status' | 'response-time' | 'json-schema' | 'contains' | 'javascript'
  config: Record<string, any>
  enabled: boolean
}

interface ValidationResult {
  ruleId: string
  passed: boolean
  message: string
}

interface TestCollection {
  id: string
  name: string
  description: string
  tests: TestRequest[]
  createdAt: Date
  updatedAt: Date
}

const httpMethods: HttpMethod[] = [
  'GET',
  'POST',
  'PUT',
  'PATCH',
  'DELETE',
  'HEAD',
  'OPTIONS'
]

const statusColors = {
  success: 'text-green-600 dark:text-green-400',
  error: 'text-red-600 dark:text-red-400',
  warning: 'text-yellow-600 dark:text-yellow-400',
  info: 'text-blue-600 dark:text-blue-400'
}

const getStatusColor = (status: number) => {
  if (status >= 200 && status < 300) return statusColors.success
  if (status >= 400 && status < 500) return statusColors.warning
  if (status >= 500) return statusColors.error
  return statusColors.info
}

export default function IntegrationTestingPage() {
  const { toast } = useToast()
  const [activeTab, setActiveTab] = useState('request')
  const [isExecuting, setIsExecuting] = useState(false)
  const [currentRequest, setCurrentRequest] = useState<TestRequest>({
    id: '1',
    name: 'New Request',
    url: 'https://api.example.com/v1/resource',
    method: 'GET',
    headers: [
      { id: '1', key: 'Content-Type', value: 'application/json', enabled: true }
    ],
    queryParams: [],
    authType: 'none',
    authConfig: {},
    body: '',
    validations: []
  })
  const [testHistory, setTestHistory] = useState<TestExecution[]>([])
  const [collections, setCollections] = useState<TestCollection[]>([])
  const [showSaveDialog, setShowSaveDialog] = useState(false)
  const [showImportDialog, setShowImportDialog] = useState(false)
  const [selectedCollection, setSelectedCollection] = useState<string>('')

  // Handle request execution
  const executeRequest = async () => {
    setIsExecuting(true)
    const startTime = Date.now()

    try {
      // Build URL with query parameters
      const url = new URL(currentRequest.url)
      currentRequest.queryParams
        .filter((p) => p.enabled && p.key)
        .forEach((param) => url.searchParams.append(param.key, param.value))

      // Build headers
      const headers: Record<string, string> = {}
      currentRequest.headers
        .filter((h) => h.enabled && h.key)
        .forEach((header) => {
          headers[header.key] = header.value
        })

      // Add authentication headers
      switch (currentRequest.authType) {
        case 'bearer':
          headers['Authorization'] = `Bearer ${currentRequest.authConfig.token}`
          break
        case 'api-key':
          headers[currentRequest.authConfig.headerName || 'X-API-Key'] =
            currentRequest.authConfig.apiKey
          break
        case 'basic':
          const credentials = btoa(
            `${currentRequest.authConfig.username}:${currentRequest.authConfig.password}`
          )
          headers['Authorization'] = `Basic ${credentials}`
          break
      }

      // Mock response for demo
      await new Promise((resolve) =>
        setTimeout(resolve, 500 + Math.random() * 1000)
      )

      const mockResponse: TestResponse = {
        status: 200,
        statusText: 'OK',
        headers: {
          'content-type': 'application/json',
          'x-request-id': 'req_' + Date.now(),
          'cache-control': 'no-cache',
          date: new Date().toUTCString()
        },
        body: {
          success: true,
          data: {
            id: 'resource_123',
            name: 'Test Resource',
            status: 'active',
            created: new Date().toISOString()
          },
          meta: {
            version: '1.0.0',
            timestamp: Date.now()
          }
        },
        time: Date.now() - startTime,
        size: 1234
      }

      // Run validations
      const validationResults = runValidations(mockResponse)
      const passed = validationResults.every((r) => r.passed)

      // Add to history
      const execution: TestExecution = {
        id: Date.now().toString(),
        request: { ...currentRequest },
        response: mockResponse,
        timestamp: new Date(),
        passed,
        validationResults
      }

      setTestHistory([execution, ...testHistory.slice(0, 49)])
      setActiveTab('response')

      toast({
        title: 'Request executed',
        description: `${currentRequest.method} ${currentRequest.url} - ${mockResponse.status} ${mockResponse.statusText}`,
        variant: passed ? 'default' : 'destructive'
      })
    } catch (error) {
      toast({
        title: 'Request failed',
        description:
          error instanceof Error ? error.message : 'An error occurred',
        variant: 'destructive'
      })
    } finally {
      setIsExecuting(false)
    }
  }

  // Run validation rules
  const runValidations = (response: TestResponse): ValidationResult[] => {
    return currentRequest.validations
      .filter((v) => v.enabled)
      .map((validation) => {
        switch (validation.type) {
          case 'status':
            const expectedStatus = validation.config.status as number
            return {
              ruleId: validation.id,
              passed: response.status === expectedStatus,
              message: `Expected status ${expectedStatus}, got ${response.status}`
            }

          case 'response-time':
            const maxTime = validation.config.maxTime as number
            return {
              ruleId: validation.id,
              passed: response.time <= maxTime,
              message: `Response time ${response.time}ms ${response.time <= maxTime ? '≤' : '>'} ${maxTime}ms`
            }

          case 'contains':
            const searchText = validation.config.text as string
            const bodyString = JSON.stringify(response.body)
            const contains = bodyString.includes(searchText)
            return {
              ruleId: validation.id,
              passed: contains,
              message: contains
                ? `Response contains "${searchText}"`
                : `Response does not contain "${searchText}"`
            }

          default:
            return {
              ruleId: validation.id,
              passed: true,
              message: 'Validation not implemented'
            }
        }
      })
  }

  // Add/remove headers
  const addHeader = () => {
    setCurrentRequest({
      ...currentRequest,
      headers: [
        ...currentRequest.headers,
        { id: Date.now().toString(), key: '', value: '', enabled: true }
      ]
    })
  }

  const removeHeader = (id: string) => {
    setCurrentRequest({
      ...currentRequest,
      headers: currentRequest.headers.filter((h) => h.id !== id)
    })
  }

  const updateHeader = (id: string, updates: Partial<Header>) => {
    setCurrentRequest({
      ...currentRequest,
      headers: currentRequest.headers.map((h) =>
        h.id === id ? { ...h, ...updates } : h
      )
    })
  }

  // Add/remove query parameters
  const addQueryParam = () => {
    setCurrentRequest({
      ...currentRequest,
      queryParams: [
        ...currentRequest.queryParams,
        { id: Date.now().toString(), key: '', value: '', enabled: true }
      ]
    })
  }

  const removeQueryParam = (id: string) => {
    setCurrentRequest({
      ...currentRequest,
      queryParams: currentRequest.queryParams.filter((p) => p.id !== id)
    })
  }

  const updateQueryParam = (id: string, updates: Partial<QueryParam>) => {
    setCurrentRequest({
      ...currentRequest,
      queryParams: currentRequest.queryParams.map((p) =>
        p.id === id ? { ...p, ...updates } : p
      )
    })
  }

  // Add/remove validations
  const addValidation = (type: ValidationRule['type']) => {
    const newValidation: ValidationRule = {
      id: Date.now().toString(),
      name: `${type} validation`,
      type,
      config: getDefaultValidationConfig(type),
      enabled: true
    }

    setCurrentRequest({
      ...currentRequest,
      validations: [...currentRequest.validations, newValidation]
    })
  }

  const removeValidation = (id: string) => {
    setCurrentRequest({
      ...currentRequest,
      validations: currentRequest.validations.filter((v) => v.id !== id)
    })
  }

  const updateValidation = (id: string, updates: Partial<ValidationRule>) => {
    setCurrentRequest({
      ...currentRequest,
      validations: currentRequest.validations.map((v) =>
        v.id === id ? { ...v, ...updates } : v
      )
    })
  }

  const getDefaultValidationConfig = (type: ValidationRule['type']) => {
    switch (type) {
      case 'status':
        return { status: 200 }
      case 'response-time':
        return { maxTime: 1000 }
      case 'contains':
        return { text: '' }
      case 'json-schema':
        return { schema: '{}' }
      case 'javascript':
        return { code: 'return response.status === 200;' }
      default:
        return {}
    }
  }

  // Save test to collection
  const saveTestToCollection = (collectionId: string) => {
    const collection = collections.find((c) => c.id === collectionId)
    if (!collection) return

    const updatedCollection = {
      ...collection,
      tests: [
        ...collection.tests,
        { ...currentRequest, id: Date.now().toString() }
      ],
      updatedAt: new Date()
    }

    setCollections(
      collections.map((c) => (c.id === collectionId ? updatedCollection : c))
    )

    toast({
      title: 'Test saved',
      description: `Test saved to "${collection.name}" collection`
    })
    setShowSaveDialog(false)
  }

  // Export/Import collections
  const exportCollections = () => {
    const data = JSON.stringify(collections, null, 2)
    const blob = new Blob([data], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'workflow-integration-tests.json'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)

    toast({
      title: 'Collections exported',
      description: 'Test collections exported successfully'
    })
  }

  const latestExecution = testHistory[0]

  return (
    <div className="flex h-[calc(100vh-4rem)] gap-4 p-4">
      {/* Request Configuration */}
      <div className="flex-1 space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Integration Endpoint Testing</CardTitle>
            <CardDescription>
              Test and validate service endpoints for workflow integrations
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* URL and Method */}
            <div className="flex gap-2">
              <Select
                value={currentRequest.method}
                onValueChange={(value: HttpMethod) =>
                  setCurrentRequest({ ...currentRequest, method: value })
                }
              >
                <SelectTrigger className="w-[120px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {httpMethods.map((method) => (
                    <SelectItem key={method} value={method}>
                      {method}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                placeholder="Enter request URL"
                value={currentRequest.url}
                onChange={(e) =>
                  setCurrentRequest({ ...currentRequest, url: e.target.value })
                }
                className="flex-1"
              />
              <Button
                onClick={executeRequest}
                disabled={isExecuting || !currentRequest.url}
              >
                {isExecuting ? (
                  <>
                    <Clock className="mr-2 h-4 w-4 animate-spin" />
                    Sending...
                  </>
                ) : (
                  <>
                    <Send className="mr-2 h-4 w-4" />
                    Send
                  </>
                )}
              </Button>
            </div>

            {/* Configuration Tabs */}
            <Tabs value={activeTab} onValueChange={setActiveTab}>
              <TabsList className="grid w-full grid-cols-5">
                <TabsTrigger value="request">Request</TabsTrigger>
                <TabsTrigger value="auth">Auth</TabsTrigger>
                <TabsTrigger value="validation">Validation</TabsTrigger>
                <TabsTrigger value="response">Response</TabsTrigger>
                <TabsTrigger value="history">History</TabsTrigger>
              </TabsList>

              {/* Request Tab */}
              <TabsContent value="request" className="space-y-4">
                <Accordion type="single" collapsible defaultValue="headers">
                  {/* Headers */}
                  <AccordionItem value="headers">
                    <AccordionTrigger>
                      Headers
                      <Badge variant="secondary" className="ml-2">
                        {currentRequest.headers.filter((h) => h.enabled).length}
                      </Badge>
                    </AccordionTrigger>
                    <AccordionContent className="space-y-2">
                      {currentRequest.headers.map((header) => (
                        <div
                          key={header.id}
                          className="flex items-center gap-2"
                        >
                          <Checkbox
                            checked={header.enabled}
                            onCheckedChange={(checked) =>
                              updateHeader(header.id, {
                                enabled: checked as boolean
                              })
                            }
                          />
                          <Input
                            placeholder="Header name"
                            value={header.key}
                            onChange={(e) =>
                              updateHeader(header.id, { key: e.target.value })
                            }
                            className="flex-1"
                          />
                          <Input
                            placeholder="Header value"
                            value={header.value}
                            onChange={(e) =>
                              updateHeader(header.id, { value: e.target.value })
                            }
                            className="flex-1"
                          />
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => removeHeader(header.id)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      ))}
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={addHeader}
                        className="w-full"
                      >
                        <Plus className="mr-2 h-4 w-4" />
                        Add Header
                      </Button>
                    </AccordionContent>
                  </AccordionItem>

                  {/* Query Parameters */}
                  <AccordionItem value="params">
                    <AccordionTrigger>
                      Query Parameters
                      <Badge variant="secondary" className="ml-2">
                        {
                          currentRequest.queryParams.filter((p) => p.enabled)
                            .length
                        }
                      </Badge>
                    </AccordionTrigger>
                    <AccordionContent className="space-y-2">
                      {currentRequest.queryParams.map((param) => (
                        <div key={param.id} className="flex items-center gap-2">
                          <Checkbox
                            checked={param.enabled}
                            onCheckedChange={(checked) =>
                              updateQueryParam(param.id, {
                                enabled: checked as boolean
                              })
                            }
                          />
                          <Input
                            placeholder="Parameter name"
                            value={param.key}
                            onChange={(e) =>
                              updateQueryParam(param.id, {
                                key: e.target.value
                              })
                            }
                            className="flex-1"
                          />
                          <Input
                            placeholder="Parameter value"
                            value={param.value}
                            onChange={(e) =>
                              updateQueryParam(param.id, {
                                value: e.target.value
                              })
                            }
                            className="flex-1"
                          />
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => removeQueryParam(param.id)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      ))}
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={addQueryParam}
                        className="w-full"
                      >
                        <Plus className="mr-2 h-4 w-4" />
                        Add Parameter
                      </Button>
                    </AccordionContent>
                  </AccordionItem>

                  {/* Request Body */}
                  {['POST', 'PUT', 'PATCH'].includes(currentRequest.method) && (
                    <AccordionItem value="body">
                      <AccordionTrigger>Request Body</AccordionTrigger>
                      <AccordionContent>
                        <div className="h-[300px] overflow-hidden rounded-md border">
                          <Editor
                            defaultLanguage="json"
                            value={currentRequest.body}
                            onChange={(value) =>
                              setCurrentRequest({
                                ...currentRequest,
                                body: value || ''
                              })
                            }
                            options={{
                              minimap: { enabled: false },
                              fontSize: 14,
                              lineNumbers: 'on',
                              scrollBeyondLastLine: false,
                              automaticLayout: true
                            }}
                            theme="vs-dark"
                          />
                        </div>
                      </AccordionContent>
                    </AccordionItem>
                  )}
                </Accordion>
              </TabsContent>

              {/* Auth Tab */}
              <TabsContent value="auth" className="space-y-4">
                <div className="space-y-2">
                  <Label>Authentication Type</Label>
                  <Select
                    value={currentRequest.authType}
                    onValueChange={(value: AuthType) =>
                      setCurrentRequest({
                        ...currentRequest,
                        authType: value,
                        authConfig: {}
                      })
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">No Authentication</SelectItem>
                      <SelectItem value="bearer">Bearer Token</SelectItem>
                      <SelectItem value="api-key">API Key</SelectItem>
                      <SelectItem value="basic">Basic Auth</SelectItem>
                      <SelectItem value="oauth2">OAuth 2.0</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {/* Auth Configuration */}
                {currentRequest.authType === 'bearer' && (
                  <div className="space-y-2">
                    <Label>Bearer Token</Label>
                    <Input
                      placeholder="Enter bearer token"
                      value={currentRequest.authConfig.token || ''}
                      onChange={(e) =>
                        setCurrentRequest({
                          ...currentRequest,
                          authConfig: {
                            ...currentRequest.authConfig,
                            token: e.target.value
                          }
                        })
                      }
                    />
                  </div>
                )}

                {currentRequest.authType === 'api-key' && (
                  <>
                    <div className="space-y-2">
                      <Label>Header Name</Label>
                      <Input
                        placeholder="X-API-Key"
                        value={currentRequest.authConfig.headerName || ''}
                        onChange={(e) =>
                          setCurrentRequest({
                            ...currentRequest,
                            authConfig: {
                              ...currentRequest.authConfig,
                              headerName: e.target.value
                            }
                          })
                        }
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>API Key</Label>
                      <Input
                        placeholder="Enter API key"
                        value={currentRequest.authConfig.apiKey || ''}
                        onChange={(e) =>
                          setCurrentRequest({
                            ...currentRequest,
                            authConfig: {
                              ...currentRequest.authConfig,
                              apiKey: e.target.value
                            }
                          })
                        }
                      />
                    </div>
                  </>
                )}

                {currentRequest.authType === 'basic' && (
                  <>
                    <div className="space-y-2">
                      <Label>Username</Label>
                      <Input
                        placeholder="Enter username"
                        value={currentRequest.authConfig.username || ''}
                        onChange={(e) =>
                          setCurrentRequest({
                            ...currentRequest,
                            authConfig: {
                              ...currentRequest.authConfig,
                              username: e.target.value
                            }
                          })
                        }
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Password</Label>
                      <Input
                        type="password"
                        placeholder="Enter password"
                        value={currentRequest.authConfig.password || ''}
                        onChange={(e) =>
                          setCurrentRequest({
                            ...currentRequest,
                            authConfig: {
                              ...currentRequest.authConfig,
                              password: e.target.value
                            }
                          })
                        }
                      />
                    </div>
                  </>
                )}
              </TabsContent>

              {/* Validation Tab */}
              <TabsContent value="validation" className="space-y-4">
                <div className="space-y-2">
                  {currentRequest.validations.map((validation) => (
                    <Card key={validation.id}>
                      <CardContent className="pt-4">
                        <div className="mb-2 flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <Checkbox
                              checked={validation.enabled}
                              onCheckedChange={(checked) =>
                                updateValidation(validation.id, {
                                  enabled: checked as boolean
                                })
                              }
                            />
                            <Input
                              value={validation.name}
                              onChange={(e) =>
                                updateValidation(validation.id, {
                                  name: e.target.value
                                })
                              }
                              className="w-[200px]"
                            />
                            <Badge variant="outline">{validation.type}</Badge>
                          </div>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => removeValidation(validation.id)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>

                        {/* Validation Configuration */}
                        {validation.type === 'status' && (
                          <div className="flex items-center gap-2">
                            <Label>Expected Status:</Label>
                            <Input
                              type="number"
                              value={validation.config.status}
                              onChange={(e) =>
                                updateValidation(validation.id, {
                                  config: {
                                    ...validation.config,
                                    status: parseInt(e.target.value)
                                  }
                                })
                              }
                              className="w-[100px]"
                            />
                          </div>
                        )}

                        {validation.type === 'response-time' && (
                          <div className="flex items-center gap-2">
                            <Label>Max Response Time (ms):</Label>
                            <Input
                              type="number"
                              value={validation.config.maxTime}
                              onChange={(e) =>
                                updateValidation(validation.id, {
                                  config: {
                                    ...validation.config,
                                    maxTime: parseInt(e.target.value)
                                  }
                                })
                              }
                              className="w-[100px]"
                            />
                          </div>
                        )}

                        {validation.type === 'contains' && (
                          <div className="space-y-2">
                            <Label>Response should contain:</Label>
                            <Input
                              value={validation.config.text}
                              onChange={(e) =>
                                updateValidation(validation.id, {
                                  config: {
                                    ...validation.config,
                                    text: e.target.value
                                  }
                                })
                              }
                              placeholder="Text to search for"
                            />
                          </div>
                        )}
                      </CardContent>
                    </Card>
                  ))}
                </div>

                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => addValidation('status')}
                  >
                    <CheckCircle className="mr-2 h-4 w-4" />
                    Status Code
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => addValidation('response-time')}
                  >
                    <Clock className="mr-2 h-4 w-4" />
                    Response Time
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => addValidation('contains')}
                  >
                    <FileText className="mr-2 h-4 w-4" />
                    Contains Text
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => addValidation('json-schema')}
                  >
                    <Code className="mr-2 h-4 w-4" />
                    JSON Schema
                  </Button>
                </div>
              </TabsContent>

              {/* Response Tab */}
              <TabsContent value="response" className="space-y-4">
                {latestExecution ? (
                  <>
                    {/* Response Summary */}
                    <Card>
                      <CardContent className="pt-4">
                        <div className="grid grid-cols-4 gap-4">
                          <div>
                            <p className="text-sm text-muted-foreground">
                              Status
                            </p>
                            <p
                              className={`text-2xl font-bold ${getStatusColor(latestExecution.response.status)}`}
                            >
                              {latestExecution.response.status}
                            </p>
                            <p className="text-sm text-muted-foreground">
                              {latestExecution.response.statusText}
                            </p>
                          </div>
                          <div>
                            <p className="text-sm text-muted-foreground">
                              Time
                            </p>
                            <p className="text-2xl font-bold">
                              {latestExecution.response.time}ms
                            </p>
                          </div>
                          <div>
                            <p className="text-sm text-muted-foreground">
                              Size
                            </p>
                            <p className="text-2xl font-bold">
                              {(latestExecution.response.size / 1024).toFixed(
                                2
                              )}
                              KB
                            </p>
                          </div>
                          <div>
                            <p className="text-sm text-muted-foreground">
                              Tests
                            </p>
                            <div className="flex items-center gap-2">
                              <p className="text-2xl font-bold">
                                {
                                  latestExecution.validationResults.filter(
                                    (r) => r.passed
                                  ).length
                                }
                                /{latestExecution.validationResults.length}
                              </p>
                              {latestExecution.passed ? (
                                <CheckCircle className="h-5 w-5 text-green-500" />
                              ) : (
                                <XCircle className="h-5 w-5 text-red-500" />
                              )}
                            </div>
                          </div>
                        </div>
                      </CardContent>
                    </Card>

                    {/* Validation Results */}
                    {latestExecution.validationResults.length > 0 && (
                      <Card>
                        <CardHeader>
                          <CardTitle className="text-base">
                            Validation Results
                          </CardTitle>
                        </CardHeader>
                        <CardContent className="space-y-2">
                          {latestExecution.validationResults.map(
                            (result, index) => {
                              const rule = currentRequest.validations.find(
                                (v) => v.id === result.ruleId
                              )
                              return (
                                <div
                                  key={index}
                                  className="flex items-center justify-between"
                                >
                                  <div className="flex items-center gap-2">
                                    {result.passed ? (
                                      <CheckCircle className="h-4 w-4 text-green-500" />
                                    ) : (
                                      <XCircle className="h-4 w-4 text-red-500" />
                                    )}
                                    <span className="font-medium">
                                      {rule?.name || 'Unknown'}
                                    </span>
                                  </div>
                                  <span className="text-sm text-muted-foreground">
                                    {result.message}
                                  </span>
                                </div>
                              )
                            }
                          )}
                        </CardContent>
                      </Card>
                    )}

                    {/* Response Headers */}
                    <Accordion type="single" collapsible defaultValue="body">
                      <AccordionItem value="headers">
                        <AccordionTrigger>Response Headers</AccordionTrigger>
                        <AccordionContent>
                          <div className="space-y-1">
                            {Object.entries(
                              latestExecution.response.headers
                            ).map(([key, value]) => (
                              <div key={key} className="flex gap-2 text-sm">
                                <span className="font-medium">{key}:</span>
                                <span className="text-muted-foreground">
                                  {value}
                                </span>
                              </div>
                            ))}
                          </div>
                        </AccordionContent>
                      </AccordionItem>

                      {/* Response Body */}
                      <AccordionItem value="body">
                        <AccordionTrigger>Response Body</AccordionTrigger>
                        <AccordionContent>
                          <div className="h-[400px] overflow-hidden rounded-md border">
                            <Editor
                              defaultLanguage="json"
                              value={JSON.stringify(
                                latestExecution.response.body,
                                null,
                                2
                              )}
                              options={{
                                readOnly: true,
                                minimap: { enabled: false },
                                fontSize: 14,
                                lineNumbers: 'on',
                                scrollBeyondLastLine: false,
                                automaticLayout: true
                              }}
                              theme="vs-dark"
                            />
                          </div>
                        </AccordionContent>
                      </AccordionItem>
                    </Accordion>
                  </>
                ) : (
                  <div className="flex flex-col items-center justify-center py-12 text-center">
                    <Zap className="mb-4 h-12 w-12 text-muted-foreground" />
                    <h3 className="mb-2 text-lg font-medium">
                      No response yet
                    </h3>
                    <p className="text-sm text-muted-foreground">
                      Send a request to see the response here
                    </p>
                  </div>
                )}
              </TabsContent>

              {/* History Tab */}
              <TabsContent value="history" className="space-y-4">
                <ScrollArea className="h-[500px]">
                  {testHistory.length > 0 ? (
                    <div className="space-y-2">
                      {testHistory.map((execution) => (
                        <Card
                          key={execution.id}
                          className="cursor-pointer hover:bg-muted/50"
                          onClick={() => {
                            setCurrentRequest(execution.request)
                            setActiveTab('response')
                          }}
                        >
                          <CardContent className="pt-4">
                            <div className="flex items-center justify-between">
                              <div className="flex items-center gap-3">
                                <Badge variant="outline">
                                  {execution.request.method}
                                </Badge>
                                <span className="text-sm font-medium">
                                  {new URL(execution.request.url).pathname}
                                </span>
                                <span
                                  className={`text-sm font-medium ${getStatusColor(execution.response.status)}`}
                                >
                                  {execution.response.status}
                                </span>
                              </div>
                              <div className="flex items-center gap-3 text-sm text-muted-foreground">
                                <span>{execution.response.time}ms</span>
                                <span>
                                  {new Date(
                                    execution.timestamp
                                  ).toLocaleTimeString()}
                                </span>
                                {execution.passed ? (
                                  <CheckCircle className="h-4 w-4 text-green-500" />
                                ) : (
                                  <XCircle className="h-4 w-4 text-red-500" />
                                )}
                              </div>
                            </div>
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  ) : (
                    <div className="flex flex-col items-center justify-center py-12 text-center">
                      <History className="mb-4 h-12 w-12 text-muted-foreground" />
                      <h3 className="mb-2 text-lg font-medium">
                        No test history
                      </h3>
                      <p className="text-sm text-muted-foreground">
                        Your test execution history will appear here
                      </p>
                    </div>
                  )}
                </ScrollArea>
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>
      </div>

      {/* Test Collections Sidebar */}
      <div className="w-[300px] space-y-4">
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="text-base">Test Collections</CardTitle>
              <div className="flex gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowImportDialog(true)}
                >
                  <Upload className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={exportCollections}
                  disabled={collections.length === 0}
                >
                  <Download className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-[400px]">
              {collections.length > 0 ? (
                <div className="space-y-2">
                  {collections.map((collection) => (
                    <Card
                      key={collection.id}
                      className="cursor-pointer hover:bg-muted/50"
                    >
                      <CardContent className="pt-4">
                        <div className="space-y-2">
                          <div className="flex items-center justify-between">
                            <h4 className="font-medium">{collection.name}</h4>
                            <Badge variant="secondary">
                              {collection.tests.length}
                            </Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">
                            {collection.description}
                          </p>
                          <div className="flex gap-1">
                            <Button
                              variant="outline"
                              size="sm"
                              className="flex-1"
                              onClick={() => {
                                // Run all tests in collection
                                toast({
                                  title: 'Running collection',
                                  description: `Running ${collection.tests.length} tests...`
                                })
                              }}
                            >
                              <Play className="mr-2 h-3 w-3" />
                              Run All
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => {
                                setSelectedCollection(collection.id)
                                setShowSaveDialog(true)
                              }}
                            >
                              <Plus className="h-3 w-3" />
                            </Button>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              ) : (
                <div className="flex flex-col items-center justify-center py-8 text-center">
                  <FolderOpen className="mb-2 h-8 w-8 text-muted-foreground" />
                  <p className="text-sm text-muted-foreground">
                    No test collections yet
                  </p>
                  <Button
                    variant="outline"
                    size="sm"
                    className="mt-2"
                    onClick={() => {
                      const newCollection: TestCollection = {
                        id: Date.now().toString(),
                        name: 'New Collection',
                        description: 'A new test collection',
                        tests: [],
                        createdAt: new Date(),
                        updatedAt: new Date()
                      }
                      setCollections([newCollection])
                    }}
                  >
                    Create Collection
                  </Button>
                </div>
              )}
            </ScrollArea>

            {/* Quick Actions */}
            <Separator className="my-4" />
            <div className="space-y-2">
              <Button
                variant="outline"
                size="sm"
                className="w-full"
                onClick={() => setShowSaveDialog(true)}
                disabled={!currentRequest.url}
              >
                <Save className="mr-2 h-4 w-4" />
                Save Current Test
              </Button>
            </div>
          </CardContent>
        </Card>

        {/* Test Templates */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Quick Templates</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Button
              variant="outline"
              size="sm"
              className="w-full justify-start"
              onClick={() => {
                setCurrentRequest({
                  ...currentRequest,
                  url: 'http://midaz-onboarding:3000/v1/organizations/{organizationId}/accounts',
                  method: 'GET',
                  headers: [
                    {
                      id: '1',
                      key: 'Authorization',
                      value: 'Bearer {token}',
                      enabled: true
                    }
                  ]
                })
              }}
            >
              <Database className="mr-2 h-4 w-4" />
              Get Accounts
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="w-full justify-start"
              onClick={() => {
                setCurrentRequest({
                  ...currentRequest,
                  url: 'http://midaz-transaction:3001/v1/organizations/{organizationId}/transactions',
                  method: 'POST',
                  headers: [
                    {
                      id: '1',
                      key: 'Content-Type',
                      value: 'application/json',
                      enabled: true
                    },
                    {
                      id: '2',
                      key: 'Authorization',
                      value: 'Bearer {token}',
                      enabled: true
                    }
                  ],
                  body: JSON.stringify(
                    {
                      send: {
                        source: {
                          from: '{sourceAccountId}',
                          amount: 1000,
                          asset: 'USD'
                        },
                        destination: {
                          to: '{destinationAccountId}',
                          amount: 1000,
                          asset: 'USD'
                        }
                      }
                    },
                    null,
                    2
                  )
                })
              }}
            >
              <Zap className="mr-2 h-4 w-4" />
              Create Transaction
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="w-full justify-start"
              onClick={() => {
                setCurrentRequest({
                  ...currentRequest,
                  url: 'http://plugin-fees:4002/v1/fees/calculate',
                  method: 'POST',
                  headers: [
                    {
                      id: '1',
                      key: 'Content-Type',
                      value: 'application/json',
                      enabled: true
                    }
                  ],
                  body: JSON.stringify(
                    {
                      amount: 10000,
                      transactionType: 'transfer',
                      currency: 'USD'
                    },
                    null,
                    2
                  )
                })
              }}
            >
              <DollarSign className="mr-2 h-4 w-4" />
              Calculate Fees
            </Button>
          </CardContent>
        </Card>
      </div>

      {/* Save Test Dialog */}
      <Dialog open={showSaveDialog} onOpenChange={setShowSaveDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Save Test to Collection</DialogTitle>
            <DialogDescription>
              Save the current test configuration to a collection for later use
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Test Name</Label>
              <Input
                value={currentRequest.name}
                onChange={(e) =>
                  setCurrentRequest({ ...currentRequest, name: e.target.value })
                }
                placeholder="Enter test name"
              />
            </div>
            <div className="space-y-2">
              <Label>Collection</Label>
              <Select
                value={selectedCollection}
                onValueChange={setSelectedCollection}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select collection" />
                </SelectTrigger>
                <SelectContent>
                  {collections.map((collection) => (
                    <SelectItem key={collection.id} value={collection.id}>
                      {collection.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSaveDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => saveTestToCollection(selectedCollection)}
              disabled={!selectedCollection || !currentRequest.name}
            >
              Save Test
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
