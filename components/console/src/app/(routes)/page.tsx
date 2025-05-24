'use client'

import { useListOrganizations } from '@/client/organizations'
import { PageHeader } from '@/components/page-header'
import { useIntl } from 'react-intl'
import { useSession } from 'next-auth/react'
import { PageContent, PageRoot, PageView } from '@/components/page'
import { Sidebar } from '@/components/sidebar'
import { Header } from '@/components/header'
import { CRMDashboardWidget } from '@/components/crm/crm-dashboard-widget'

const Page = () => {
  const intl = useIntl()
  const { data: session } = useSession()
  const { data, isLoading } = useListOrganizations({})
  const hasOrganizations = data?.items.length! > 0

  if (isLoading) {
    return null
  }

  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <PageContent>
          {hasOrganizations && (
            <>
              <PageHeader.Root>
                <PageHeader.InfoTitle
                  title={intl.formatMessage(
                    {
                      id: 'homePage.welcome.title',
                      defaultMessage: 'Welcome, {user}!'
                    },
                    {
                      user: (session?.user?.name as string) || 'Guest'
                    }
                  )}
                  subtitle={intl.formatMessage({
                    id: 'homePage.welcome.subtitle',
                    defaultMessage:
                      "Here's an overview of your organization's activity."
                  })}
                />
              </PageHeader.Root>

              {/* CRM Dashboard Widget */}
              <div className="mt-8">
                <CRMDashboardWidget />
              </div>
            </>
          )}
        </PageContent>
      </PageView>
    </PageRoot>
  )
}

export default Page
