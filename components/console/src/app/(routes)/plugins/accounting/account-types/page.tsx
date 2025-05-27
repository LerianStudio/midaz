'use client'

import { useState } from 'react'
import { Plus } from 'lucide-react'
import Link from 'next/link'

import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/page-header'
import { AccountTypeDataTable } from '@/components/accounting/account-types/account-type-data-table'
import { mockAccountTypes } from '@/core/domain/mock-data/accounting-mock-data'

export default function AccountTypesPage() {
  const [accountTypes] = useState(mockAccountTypes)

  return (
    <div className="flex h-full flex-col">
      <div className="px-6">
        <PageHeader.Root>
          <PageHeader.Wrapper>
            <PageHeader.InfoTitle
              title="Account Types"
              subtitle="Manage chart of accounts with domain validation and business rules"
            />
            <PageHeader.ActionButtons>
              <Button asChild>
                <Link href="/plugins/accounting/account-types/create">
                  <Plus className="mr-2 h-4 w-4" />
                  New Account Type
                </Link>
              </Button>
            </PageHeader.ActionButtons>
          </PageHeader.Wrapper>
        </PageHeader.Root>
      </div>

      <div className="flex-1 px-6 pb-6">
        <AccountTypeDataTable data={accountTypes} />
      </div>
    </div>
  )
}
