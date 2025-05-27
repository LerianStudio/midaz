'use client'

import React, { useState, useCallback } from 'react'
import { useDropzone } from 'react-dropzone'
import {
  Upload,
  File,
  X,
  CheckCircle,
  AlertTriangle,
  FileText,
  Eye
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { useToast } from '@/hooks/use-toast'

interface UploadedFile {
  file: File
  id: string
  status: 'uploading' | 'processing' | 'completed' | 'error'
  progress: number
  content?: string
  variables?: string[]
  error?: string
}

interface TemplateFileUploadProps {
  onFileUploaded: (file: UploadedFile) => void
  onFileRemoved: (fileId: string) => void
  maxFiles?: number
  className?: string
}

export function TemplateFileUpload({
  onFileUploaded,
  onFileRemoved,
  maxFiles = 1,
  className
}: TemplateFileUploadProps) {
  const [uploadedFiles, setUploadedFiles] = useState<UploadedFile[]>([])
  const { toast } = useToast()

  const validateTemplateFile = (
    fileContent: string
  ): { isValid: boolean; variables: string[]; error?: string } => {
    try {
      // Basic Pongo2 template validation
      const variableMatches = fileContent.match(/\{\{\s*([^}]+)\s*\}\}/g) || []
      const variables = variableMatches
        .map((match) => match.replace(/[{}]/g, '').trim())
        .filter((v, i, arr) => arr.indexOf(v) === i) // Remove duplicates

      // Check for unclosed tags
      const openTags = (fileContent.match(/\{\{/g) || []).length
      const closeTags = (fileContent.match(/\}\}/g) || []).length

      if (openTags !== closeTags) {
        return {
          isValid: false,
          variables: [],
          error: 'Unclosed template tags detected'
        }
      }

      // Check for basic syntax errors
      const forLoops = fileContent.match(/\{\%\s*for\s+.*?\%\}/g) || []
      const endForLoops = fileContent.match(/\{\%\s*endfor\s*\%\}/g) || []

      if (forLoops.length !== endForLoops.length) {
        return {
          isValid: false,
          variables: [],
          error: 'Mismatched for/endfor tags'
        }
      }

      return {
        isValid: true,
        variables,
        error: undefined
      }
    } catch (error) {
      return {
        isValid: false,
        variables: [],
        error: 'Failed to parse template'
      }
    }
  }

  const processFile = async (file: File): Promise<void> => {
    const fileId = Math.random().toString(36).substring(7)

    const uploadedFile: UploadedFile = {
      file,
      id: fileId,
      status: 'uploading',
      progress: 0
    }

    setUploadedFiles((prev) => [...prev, uploadedFile])

    // Simulate upload progress
    for (let progress = 0; progress <= 100; progress += 20) {
      await new Promise((resolve) => setTimeout(resolve, 200))
      setUploadedFiles((prev) =>
        prev.map((f) => (f.id === fileId ? { ...f, progress } : f))
      )
    }

    // Read file content
    try {
      setUploadedFiles((prev) =>
        prev.map((f) => (f.id === fileId ? { ...f, status: 'processing' } : f))
      )

      const content = await file.text()
      const validation = validateTemplateFile(content)

      if (validation.isValid) {
        const finalFile: UploadedFile = {
          ...uploadedFile,
          status: 'completed',
          progress: 100,
          content,
          variables: validation.variables
        }

        setUploadedFiles((prev) =>
          prev.map((f) => (f.id === fileId ? finalFile : f))
        )

        onFileUploaded(finalFile)

        toast({
          title: 'Template uploaded successfully',
          description: `Found ${validation.variables.length} variables in template`
        })
      } else {
        setUploadedFiles((prev) =>
          prev.map((f) =>
            f.id === fileId
              ? {
                  ...f,
                  status: 'error',
                  error: validation.error
                }
              : f
          )
        )

        toast({
          title: 'Template validation failed',
          description: validation.error,
          variant: 'destructive'
        })
      }
    } catch (error) {
      setUploadedFiles((prev) =>
        prev.map((f) =>
          f.id === fileId
            ? {
                ...f,
                status: 'error',
                error: 'Failed to read file content'
              }
            : f
        )
      )

      toast({
        title: 'Upload failed',
        description: 'Failed to read file content',
        variant: 'destructive'
      })
    }
  }

  const onDrop = useCallback(
    async (acceptedFiles: File[]) => {
      const templateFiles = acceptedFiles.filter((file) => {
        // Accept .tpl, .html, .txt files
        return file.name.match(/\.(tpl|html|htm|txt)$/i)
      })

      if (templateFiles.length === 0) {
        toast({
          title: 'Invalid file type',
          description: 'Please upload .tpl, .html, or .txt files only',
          variant: 'destructive'
        })
        return
      }

      if (uploadedFiles.length + templateFiles.length > maxFiles) {
        toast({
          title: 'Too many files',
          description: `Maximum ${maxFiles} file(s) allowed`,
          variant: 'destructive'
        })
        return
      }

      // Process each file
      for (const file of templateFiles) {
        await processFile(file)
      }
    },
    [uploadedFiles.length, maxFiles, toast]
  )

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: {
      'text/plain': ['.tpl', '.txt'],
      'text/html': ['.html', '.htm']
    },
    maxFiles,
    disabled: uploadedFiles.length >= maxFiles
  })

  const removeFile = (fileId: string) => {
    setUploadedFiles((prev) => prev.filter((f) => f.id !== fileId))
    onFileRemoved(fileId)
  }

  const getStatusIcon = (status: UploadedFile['status']) => {
    switch (status) {
      case 'uploading':
      case 'processing':
        return <Upload className="h-4 w-4 animate-pulse" />
      case 'completed':
        return <CheckCircle className="h-4 w-4 text-green-500" />
      case 'error':
        return <AlertTriangle className="h-4 w-4 text-red-500" />
      default:
        return <File className="h-4 w-4" />
    }
  }

  const getStatusColor = (status: UploadedFile['status']) => {
    switch (status) {
      case 'completed':
        return 'text-green-600'
      case 'error':
        return 'text-red-600'
      case 'uploading':
      case 'processing':
        return 'text-blue-600'
      default:
        return 'text-gray-600'
    }
  }

  return (
    <div className={className}>
      {/* Upload Area */}
      {uploadedFiles.length < maxFiles && (
        <Card>
          <CardContent className="pt-6">
            <div
              {...getRootProps()}
              className={`cursor-pointer rounded-lg border-2 border-dashed p-8 text-center transition-colors ${
                isDragActive
                  ? 'border-blue-400 bg-blue-50'
                  : 'border-gray-300 hover:border-gray-400'
              }`}
            >
              <input {...getInputProps()} />
              <Upload className="mx-auto mb-4 h-12 w-12 text-gray-400" />
              {isDragActive ? (
                <p className="text-blue-600">Drop template files here...</p>
              ) : (
                <div>
                  <p className="mb-2 text-gray-600">
                    Drop template files here, or click to browse
                  </p>
                  <p className="text-sm text-gray-500">
                    Supports .tpl, .html, and .txt files
                  </p>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Uploaded Files */}
      {uploadedFiles.length > 0 && (
        <div className="space-y-3">
          {uploadedFiles.map((uploadedFile) => (
            <Card key={uploadedFile.id}>
              <CardContent className="pt-4">
                <div className="flex items-start justify-between">
                  <div className="flex flex-1 items-start gap-3">
                    <div className="mt-1">
                      {getStatusIcon(uploadedFile.status)}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="mb-1 flex items-center gap-2">
                        <p className="truncate text-sm font-medium">
                          {uploadedFile.file.name}
                        </p>
                        <Badge variant="outline" className="text-xs">
                          {(uploadedFile.file.size / 1024).toFixed(1)} KB
                        </Badge>
                      </div>

                      <p
                        className={`text-xs ${getStatusColor(uploadedFile.status)}`}
                      >
                        {uploadedFile.status === 'uploading' && 'Uploading...'}
                        {uploadedFile.status === 'processing' &&
                          'Processing template...'}
                        {uploadedFile.status === 'completed' &&
                          `Template validated • ${uploadedFile.variables?.length || 0} variables found`}
                        {uploadedFile.status === 'error' && uploadedFile.error}
                      </p>

                      {/* Progress bar for uploading/processing */}
                      {(uploadedFile.status === 'uploading' ||
                        uploadedFile.status === 'processing') && (
                        <Progress
                          value={uploadedFile.progress}
                          className="mt-2 w-full"
                        />
                      )}

                      {/* Variables preview for completed files */}
                      {uploadedFile.status === 'completed' &&
                        uploadedFile.variables &&
                        uploadedFile.variables.length > 0 && (
                          <div className="mt-2">
                            <p className="mb-1 text-xs text-gray-600">
                              Variables found:
                            </p>
                            <div className="flex flex-wrap gap-1">
                              {uploadedFile.variables
                                .slice(0, 5)
                                .map((variable, index) => (
                                  <Badge
                                    key={index}
                                    variant="secondary"
                                    className="text-xs"
                                  >
                                    {variable}
                                  </Badge>
                                ))}
                              {uploadedFile.variables.length > 5 && (
                                <Badge variant="secondary" className="text-xs">
                                  +{uploadedFile.variables.length - 5} more
                                </Badge>
                              )}
                            </div>
                          </div>
                        )}
                    </div>
                  </div>

                  <div className="ml-3 flex items-center gap-1">
                    {uploadedFile.status === 'completed' && (
                      <Button variant="ghost" size="sm">
                        <Eye className="h-4 w-4" />
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => removeFile(uploadedFile.id)}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
