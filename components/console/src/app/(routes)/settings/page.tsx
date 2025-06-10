'use client'

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useSearchParams } from 'next/navigation'
import { Breadcrumb } from '@/components/breadcrumb'
import { useIntl } from 'react-intl'
import { useTabs } from '@/hooks/use-tabs'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { OrganizationsTabContent } from './organizations/organizations-tab-content'
import { PageHeader } from '@/components/page-header'
import { SystemTabContent } from './system-tab-content'
import React from 'react'
import { UsersTabContent } from './users/users-tab-content'
import { ApplicationsTabContent } from './applications/applications-tab-content'
import { Enforce } from '@/providers/permission-provider/enforce'

const Page = () => {
  const intl = useIntl()
  const searchParams = useSearchParams()

  const { activeTab, handleTabChange } = useTabs({
    initialValue: searchParams.get('tab') || 'organizations'
  })

  const breadcrumbPaths = [
    {
      name: intl.formatMessage({
        id: `settings.title`,
        defaultMessage: 'Settings'
      })
    },
    {
      name: intl.formatMessage({
        id: `settings.tabs.organizations`,
        defaultMessage: 'Organizations'
      }),
      active: () => activeTab === 'organizations'
    },
    {
      name: intl.formatMessage({
        id: `settings.tabs.users`,
        defaultMessage: 'Users'
      }),
      active: () => activeTab === 'users'
    },
    {
      name: intl.formatMessage({
        id: `settings.tabs.applications`,
        defaultMessage: 'Applications'
      }),
      active: () => activeTab === 'applications'
    },
    {
      name: intl.formatMessage({
        id: `settings.tabs.system`,
        defaultMessage: 'System'
      }),
      active: () => activeTab === 'system'
    }
  ]

  return (
    <React.Fragment>
      <Breadcrumb paths={getBreadcrumbPaths(breadcrumbPaths)} />

      <PageHeader.Root>
        <PageHeader.Wrapper className="border-none">
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'settings.title',
              defaultMessage: 'Settings'
            })}
          />
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <Tabs value={activeTab} onValueChange={handleTabChange}>
        <TabsList>
          <TabsTrigger value="organizations">
            {intl.formatMessage({
              id: 'settings.tabs.organizations',
              defaultMessage: 'Organizations'
            })}
          </TabsTrigger>

          <Enforce resource="users" action="get">
            <TabsTrigger value="users">
              {intl.formatMessage({
                id: 'settings.tabs.users',
                defaultMessage: 'Users'
              })}
            </TabsTrigger>
          </Enforce>

          <Enforce resource="applications" action="get">
            <TabsTrigger value="applications">
              {intl.formatMessage({
                id: 'settings.tabs.applications',
                defaultMessage: 'Applications'
              })}
            </TabsTrigger>
          </Enforce>

          <TabsTrigger value="system">
            {intl.formatMessage({
              id: 'settings.tabs.system',
              defaultMessage: 'System'
            })}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="organizations">
          <OrganizationsTabContent />
        </TabsContent>

        <Enforce resource="users" action="get">
          <TabsContent value="users">
            <UsersTabContent />
          </TabsContent>
        </Enforce>

        <Enforce resource="applications" action="get">
          <TabsContent value="applications">
            <ApplicationsTabContent />
          </TabsContent>
        </Enforce>

        <TabsContent value="system">
          <SystemTabContent />
        </TabsContent>
      </Tabs>
    </React.Fragment>
  )
}

export default Page
