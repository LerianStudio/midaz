'use client'

import { Breadcrumb } from '@/components/breadcrumb'
import { TransactionProvider } from './transaction-form-provider'
import { PageHeader } from '@/components/page-header'
import { useIntl } from 'react-intl'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'

export default function RootLayout({
  children
}: {
  children: React.ReactNode
}) {
  const intl = useIntl()

  const { currentOrganization } = useOrganization()

  return (
    <TransactionProvider>
      <Breadcrumb
        paths={[
          {
            name: currentOrganization?.legalName,
            href: '#'
          },
          {
            name: intl.formatMessage({
              id: `transactions.tab.create`,
              defaultMessage: 'New Transaction'
            })
          }
        ]}
      />

      <PageHeader.Root>
        <PageHeader.Wrapper className="border-none">
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'transactions.create.title',
              defaultMessage: 'New Transaction'
            })}
          />
        </PageHeader.Wrapper>
      </PageHeader.Root>

      {children}
    </TransactionProvider>
  )
}
