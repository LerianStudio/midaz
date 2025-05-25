'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { ArrowLeft } from 'lucide-react'
import Link from 'next/link'

import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/page-header'
import { AccountTypeWizard } from '@/components/accounting/account-types/account-type-wizard'
import { toast } from '@/hooks/use-toast'

interface AccountTypeFormData {
  name: string
  description: string
  keyValue: string
  domain: 'ledger' | 'external'
}

export default function CreateAccountTypePage() {
  const router = useRouter()
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (data: AccountTypeFormData) => {
    setIsSubmitting(true)

    try {
      // Simulate API call
      await new Promise((resolve) => setTimeout(resolve, 2000))

      // TODO: Replace with actual API call
      console.log('Creating account type:', data)

      toast({
        title: 'Account Type Created',
        description: `Account type "${data.name}" has been created successfully.`
      })

      // Navigate back to account types list
      router.push('/plugins/accounting/account-types')
    } catch (error) {
      console.error('Failed to create account type:', error)
      toast({
        title: 'Creation Failed',
        description: 'Failed to create account type. Please try again.',
        variant: 'destructive'
      })
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleCancel = () => {
    router.back()
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader.Root>
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" asChild>
            <Link href="/plugins/accounting/account-types">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <PageHeader.InfoTitle
            title="Create Account Type"
            description="Define a new account type for your chart of accounts"
          />
        </div>
        <PageHeader.InfoTooltip content="Account types are the foundation of your chart of accounts. They define how accounts behave, which validation rules apply, and whether they're managed internally or externally." />
      </PageHeader.Root>

      <div className="flex-1 px-6 pb-6">
        <AccountTypeWizard
          onSubmit={handleSubmit}
          onCancel={handleCancel}
          isSubmitting={isSubmitting}
        />
      </div>
    </div>
  )
}
