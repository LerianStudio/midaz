'use client'

import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2, Save, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@/components/ui/form'
import { DomainSelector } from './domain-selector'
import { KeyValueValidator } from './key-value-validator'
import { AccountType } from '@/core/domain/mock-data/accounting-mock-data'

const accountTypeSchema = z.object({
  name: z
    .string()
    .min(2, 'Name must be at least 2 characters')
    .max(100, 'Name must not exceed 100 characters'),
  description: z
    .string()
    .min(10, 'Description must be at least 10 characters')
    .max(500, 'Description must not exceed 500 characters'),
  keyValue: z
    .string()
    .min(2, 'Key value must be at least 2 characters')
    .max(20, 'Key value must not exceed 20 characters')
    .regex(
      /^[A-Z0-9_]+$/,
      'Key value must contain only uppercase letters, numbers, and underscores'
    ),
  domain: z.enum(['ledger', 'external'], {
    required_error: 'Please select a domain'
  })
})

type AccountTypeFormData = z.infer<typeof accountTypeSchema>

interface AccountTypeFormProps {
  accountType?: AccountType
  onSubmit: (data: AccountTypeFormData) => Promise<void>
  onCancel?: () => void
  mode?: 'create' | 'edit'
  isSubmitting?: boolean
}

export function AccountTypeForm({
  accountType,
  onSubmit,
  onCancel,
  mode = 'create',
  isSubmitting = false
}: AccountTypeFormProps) {
  const [keyValueValidation, setKeyValueValidation] = useState<{
    isValid: boolean
    error?: string
  }>({ isValid: false })

  const form = useForm<AccountTypeFormData>({
    resolver: zodResolver(accountTypeSchema),
    defaultValues: {
      name: accountType?.name || '',
      description: accountType?.description || '',
      keyValue: accountType?.keyValue || '',
      domain: accountType?.domain || undefined
    }
  })

  const {
    handleSubmit,
    formState: { errors },
    watch,
    setValue
  } = form

  const onSubmitForm = async (data: AccountTypeFormData) => {
    if (!keyValueValidation.isValid) {
      form.setError('keyValue', {
        type: 'manual',
        message: keyValueValidation.error || 'Key value validation failed'
      })
      return
    }

    await onSubmit(data)
  }

  return (
    <Form {...form}>
      <form onSubmit={handleSubmit(onSubmitForm)} className="space-y-6">
        {/* Basic Information */}
        <div className="space-y-4">
          <h3 className="text-lg font-medium">Basic Information</h3>

          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  Account Type Name <span className="text-red-500">*</span>
                </FormLabel>
                <FormControl>
                  <Input
                    placeholder="e.g., Checking Account, Savings Account"
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="description"
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  Description <span className="text-red-500">*</span>
                </FormLabel>
                <FormControl>
                  <Textarea
                    placeholder="Describe the purpose and usage of this account type..."
                    rows={3}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className="space-y-2">
            <KeyValueValidator
              value={watch('keyValue')}
              onChange={(value) => setValue('keyValue', value)}
              onValidationChange={setKeyValueValidation}
              excludeId={accountType?.id}
            />
            {errors.keyValue && (
              <p className="text-sm text-red-500">{errors.keyValue.message}</p>
            )}
          </div>
        </div>

        {/* Domain Selection */}
        <div className="space-y-4">
          <h3 className="text-lg font-medium">Domain Configuration</h3>
          <FormField
            control={form.control}
            name="domain"
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  Account Domain <span className="text-red-500">*</span>
                </FormLabel>
                <FormControl>
                  <DomainSelector
                    value={field.value}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        {/* Form Actions */}
        <div className="flex items-center justify-end gap-3 border-t pt-6">
          {onCancel && (
            <Button
              type="button"
              variant="outline"
              onClick={onCancel}
              disabled={isSubmitting}
            >
              <X className="mr-2 h-4 w-4" />
              Cancel
            </Button>
          )}
          <Button
            type="submit"
            disabled={isSubmitting || !keyValueValidation.isValid}
          >
            {isSubmitting ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Save className="mr-2 h-4 w-4" />
            )}
            {mode === 'create' ? 'Create Account Type' : 'Update Account Type'}
          </Button>
        </div>
      </form>
    </Form>
  )
}
