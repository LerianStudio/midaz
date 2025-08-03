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
import { Enforce, getRuntimeEnv } from '@lerianstudio/console-layout'

const isAuthEnabled =
    getRuntimeEnv(
      'NEXT_PUBLIC_MIDAZ_AUTH_ENABLED',
      process.env.NEXT_PUBLIC_MIDAZ_AUTH_ENABLED
    ) === 'true'

const Page = () => {
  const intl = useIntl()
  const searchParams = useSearchParams()
  const authEnabled = isAuthEnabled

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
        id: `organizations.title`,
        defaultMessage: 'Organizations'
      }),
      active: () => activeTab === 'organizations',
      
    },
    ...(authEnabled ? [
      {
        name: intl.formatMessage({
          id: `users.title`,
          defaultMessage: 'Users'
        }),
        active: () => activeTab === 'users'
      },
      {
        name: intl.formatMessage({
          id: `applications.title`,
          defaultMessage: 'Applications'
        }),
        active: () => activeTab === 'applications'
      }
    ] : []),
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
              id: 'organizations.title',
              defaultMessage: 'Organizations'
            })}
          </TabsTrigger>

          {/* Only show Users tab if auth is enabled */}
          {authEnabled && (
            <Enforce resource="users" action="get">
              <TabsTrigger value="users">
                {intl.formatMessage({
                  id: 'users.title',
                  defaultMessage: 'Users'
                })}
              </TabsTrigger>
            </Enforce>
          )}

          {/* Only show Applications tab if auth is enabled */}
          {authEnabled && (
            <Enforce resource="applications" action="get">
              <TabsTrigger value="applications">
                {intl.formatMessage({
                  id: 'applications.title',
                  defaultMessage: 'Applications'
                })}
              </TabsTrigger>
            </Enforce>
          )}

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

        {/* Only render Users tab content if auth is enabled */}
        {authEnabled && (
          <Enforce resource="users" action="get">
            <TabsContent value="users">
              <UsersTabContent />
            </TabsContent>
          </Enforce>
        )}

        {/* Only render Applications tab content if auth is enabled */}
        {authEnabled && (
          <Enforce resource="applications" action="get">
            <TabsContent value="applications">
              <ApplicationsTabContent />
            </TabsContent>
          </Enforce>
        )}

        <TabsContent value="system">
          <SystemTabContent />
        </TabsContent>
      </Tabs>
    </React.Fragment>
  )
}

export default Page
