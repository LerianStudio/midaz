'use client'

import { useState, useEffect, useCallback } from 'react'
import { Check, X, Loader2, AlertCircle, Info } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { mockAccountTypes } from '@/core/domain/mock-data/accounting-mock-data'

interface ValidationResult {
  isValid: boolean
  error?: string
  suggestions?: string[]
}

interface KeyValueValidatorProps {
  value: string
  onChange: (value: string) => void
  onValidationChange: (result: ValidationResult) => void
  disabled?: boolean
  excludeId?: string // For edit mode, exclude current account type
}

export function KeyValueValidator({
  value,
  onChange,
  onValidationChange,
  disabled = false,
  excludeId
}: KeyValueValidatorProps) {
  const [validationState, setValidationState] = useState<
    'idle' | 'validating' | 'valid' | 'invalid'
  >('idle')
  const [validationResult, setValidationResult] = useState<ValidationResult>({
    isValid: true
  })
  const [isDebouncing, setIsDebouncing] = useState(false)

  // Simulate real-time validation with debouncing
  const validateKeyValue = useCallback(
    async (keyValue: string): Promise<ValidationResult> => {
      if (!keyValue.trim()) {
        return { isValid: false, error: 'Key value is required' }
      }

      // Validate format (alphanumeric and underscores, uppercase)
      const formatRegex = /^[A-Z0-9_]+$/
      if (!formatRegex.test(keyValue)) {
        return {
          isValid: false,
          error:
            'Key value must contain only uppercase letters, numbers, and underscores',
          suggestions: [keyValue.toUpperCase().replace(/[^A-Z0-9_]/g, '_')]
        }
      }

      // Check length
      if (keyValue.length < 2) {
        return {
          isValid: false,
          error: 'Key value must be at least 2 characters long'
        }
      }

      if (keyValue.length > 20) {
        return {
          isValid: false,
          error: 'Key value must be no more than 20 characters long'
        }
      }

      // Check uniqueness against existing account types
      const existingAccountType = mockAccountTypes.find(
        (type) => type.keyValue === keyValue && type.id !== excludeId
      )

      if (existingAccountType) {
        return {
          isValid: false,
          error: `Key value "${keyValue}" is already used by "${existingAccountType.name}"`,
          suggestions: [`${keyValue}_NEW`, `${keyValue}_V2`, `NEW_${keyValue}`]
        }
      }

      // Check for reserved keywords
      const reservedKeywords = [
        'SYSTEM',
        'ADMIN',
        'ROOT',
        'DEFAULT',
        'NULL',
        'UNDEFINED'
      ]
      if (reservedKeywords.includes(keyValue)) {
        return {
          isValid: false,
          error: `"${keyValue}" is a reserved keyword and cannot be used`,
          suggestions: [`${keyValue}_ACC`, `${keyValue}_TYPE`]
        }
      }

      return { isValid: true }
    },
    [excludeId]
  )

  // Debounced validation effect
  useEffect(() => {
    if (!value) {
      setValidationState('idle')
      setValidationResult({ isValid: false, error: 'Key value is required' })
      onValidationChange({ isValid: false, error: 'Key value is required' })
      return
    }

    setIsDebouncing(true)
    const debounceTimer = setTimeout(async () => {
      setIsDebouncing(false)
      setValidationState('validating')

      try {
        // Simulate network delay
        await new Promise((resolve) => setTimeout(resolve, 300))

        const result = await validateKeyValue(value)
        setValidationResult(result)
        setValidationState(result.isValid ? 'valid' : 'invalid')
        onValidationChange(result)
      } catch (error) {
        const errorResult = {
          isValid: false,
          error: 'Validation failed. Please try again.'
        }
        setValidationResult(errorResult)
        setValidationState('invalid')
        onValidationChange(errorResult)
      }
    }, 500)

    return () => {
      clearTimeout(debounceTimer)
      setIsDebouncing(false)
    }
  }, [value, validateKeyValue, onValidationChange])

  const getValidationIcon = () => {
    if (isDebouncing || validationState === 'validating') {
      return <Loader2 className="h-4 w-4 animate-spin text-blue-500" />
    }

    if (validationState === 'valid') {
      return <Check className="h-4 w-4 text-green-500" />
    }

    if (validationState === 'invalid') {
      return <X className="h-4 w-4 text-red-500" />
    }

    return null
  }

  const handleSuggestionClick = (suggestion: string) => {
    onChange(suggestion)
  }

  return (
    <div className="space-y-3">
      <div className="space-y-2">
        <Label htmlFor="keyValue">
          Key Value <span className="text-red-500">*</span>
        </Label>
        <div className="relative">
          <Input
            id="keyValue"
            value={value}
            onChange={(e) => onChange(e.target.value.toUpperCase())}
            placeholder="e.g., CHCK, SVGS, EXT_BANK"
            disabled={disabled}
            className={`pr-10 ${
              validationState === 'valid'
                ? 'border-green-500 focus:border-green-500'
                : validationState === 'invalid'
                  ? 'border-red-500 focus:border-red-500'
                  : ''
            }`}
          />
          <div className="absolute right-3 top-1/2 -translate-y-1/2 transform">
            {getValidationIcon()}
          </div>
        </div>
      </div>

      {/* Validation Messages */}
      {validationState === 'invalid' && validationResult.error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{validationResult.error}</AlertDescription>
        </Alert>
      )}

      {validationState === 'valid' && (
        <Alert>
          <Check className="h-4 w-4" />
          <AlertDescription className="text-green-700">
            Key value is available and follows the correct format.
          </AlertDescription>
        </Alert>
      )}

      {/* Suggestions */}
      {validationResult.suggestions &&
        validationResult.suggestions.length > 0 && (
          <div className="space-y-2">
            <Label className="text-sm text-gray-600">
              Suggested alternatives:
            </Label>
            <div className="flex flex-wrap gap-2">
              {validationResult.suggestions.map((suggestion, index) => (
                <button
                  key={index}
                  type="button"
                  onClick={() => handleSuggestionClick(suggestion)}
                  className="rounded-md bg-blue-50 px-3 py-1 text-sm text-blue-700 transition-colors hover:bg-blue-100"
                  disabled={disabled}
                >
                  {suggestion}
                </button>
              ))}
            </div>
          </div>
        )}

      {/* Format Guidelines */}
      {(!value || validationState === 'idle') && (
        <Alert>
          <Info className="h-4 w-4" />
          <AlertDescription>
            <div className="space-y-1">
              <div className="font-medium">Key Value Guidelines:</div>
              <ul className="space-y-1 text-sm">
                <li>• Use uppercase letters, numbers, and underscores only</li>
                <li>• 2-20 characters in length</li>
                <li>• Must be unique across all account types</li>
                <li>• Examples: CHCK, SAVINGS, EXT_BANK, CREDIT_CARD</li>
              </ul>
            </div>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
