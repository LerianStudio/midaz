'use client'

import React from 'react'
import { useRouter } from 'next/navigation'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { PackageWizard } from '@/components/fees/packages/package-wizard'
import { CreatePackageFormData } from '@/components/fees/types/fee-types'
import { v4 as uuidv4 } from 'uuid'
import { useToast } from '@/hooks/use-toast'
import { Card } from '@/components/ui/card'

export default function CreatePackagePage() {
  const intl = useIntl()
  const router = useRouter()
  const { toast } = useToast()
  const [isSubmitting, setIsSubmitting] = React.useState(false)

  const handleCreate = async (data: CreatePackageFormData) => {
    setIsSubmitting(true)

    try {
      // Mock API call - in real app would call the API
      await new Promise((resolve) => setTimeout(resolve, 1000))

      // Simulate successful creation
      const newPackage = {
        id: uuidv4(),
        ...data,
        ledgerId: 'main-ledger',
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString()
      }

      console.log('Created package:', newPackage)

      toast({
        title: intl.formatMessage({
          id: 'fees.packages.createSuccess',
          defaultMessage: 'Package created successfully'
        }),
        description: intl.formatMessage(
          {
            id: 'fees.packages.createSuccessDescription',
            defaultMessage: 'The fee package "{name}" has been created.'
          },
          { name: data.name }
        ),
        variant: 'success'
      })

      // Redirect to packages list
      router.push('/plugins/fees/packages')
    } catch (error) {
      toast({
        title: intl.formatMessage({
          id: 'fees.packages.createError',
          defaultMessage: 'Failed to create package'
        }),
        description: intl.formatMessage({
          id: 'fees.packages.createErrorDescription',
          defaultMessage:
            'An error occurred while creating the fee package. Please try again.'
        }),
        variant: 'destructive'
      })
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleCancel = () => {
    router.push('/plugins/fees/packages')
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <PageHeader.InfoTitle
          title={intl.formatMessage({
            id: 'fees.packages.create.title',
            defaultMessage: 'Create Fee Package'
          })}
          subtitle={intl.formatMessage({
            id: 'fees.packages.create.subtitle',
            defaultMessage:
              'Set up a new fee calculation package with custom rules'
          })}
        />
      </PageHeader.Root>

      <Card className="p-6">
        <PackageWizard
          onSubmit={handleCreate}
          onCancel={handleCancel}
          isSubmitting={isSubmitting}
        />
      </Card>
    </div>
  )
}
