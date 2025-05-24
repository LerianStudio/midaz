'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import {
  Users,
  UserPlus,
  Building,
  Link as LinkIcon,
  ArrowRight,
  TrendingUp
} from 'lucide-react'
import { useRouter } from 'next/navigation'

const CRMDashboardPage = () => {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization } = useOrganization()

  const breadcrumbPaths = getBreadcrumbPaths([
    {
      name: currentOrganization.legalName
    },
    {
      name: intl.formatMessage({
        id: 'plugins.title',
        defaultMessage: 'Native Plugins'
      }),
      href: '/plugins'
    },
    {
      name: intl.formatMessage({
        id: 'crm.title',
        defaultMessage: 'CRM'
      })
    }
  ])

  // Mock stats data
  const stats = [
    {
      title: intl.formatMessage({
        id: 'crm.stats.totalCustomers',
        defaultMessage: 'Total Customers'
      }),
      value: '1,247',
      change: '+12%',
      changeType: 'positive' as const,
      icon: <Users className="h-4 w-4" />
    },
    {
      title: intl.formatMessage({
        id: 'crm.stats.individualCustomers',
        defaultMessage: 'Individual Customers'
      }),
      value: '892',
      change: '+8%',
      changeType: 'positive' as const,
      icon: <UserPlus className="h-4 w-4" />
    },
    {
      title: intl.formatMessage({
        id: 'crm.stats.corporateCustomers',
        defaultMessage: 'Corporate Customers'
      }),
      value: '355',
      change: '+24%',
      changeType: 'positive' as const,
      icon: <Building className="h-4 w-4" />
    },
    {
      title: intl.formatMessage({
        id: 'crm.stats.accountLinks',
        defaultMessage: 'Account Links'
      }),
      value: '2,103',
      change: '+15%',
      changeType: 'positive' as const,
      icon: <LinkIcon className="h-4 w-4" />
    }
  ]

  const quickActions = [
    {
      title: intl.formatMessage({
        id: 'crm.quickActions.newCustomer',
        defaultMessage: 'New Individual Customer'
      }),
      description: intl.formatMessage({
        id: 'crm.quickActions.newCustomer.description',
        defaultMessage: 'Create a new individual customer profile'
      }),
      action: () => router.push('/plugins/crm/customers/create?type=natural'),
      icon: <UserPlus className="h-5 w-5" />
    },
    {
      title: intl.formatMessage({
        id: 'crm.quickActions.newCompany',
        defaultMessage: 'New Corporate Customer'
      }),
      description: intl.formatMessage({
        id: 'crm.quickActions.newCompany.description',
        defaultMessage: 'Create a new corporate customer profile'
      }),
      action: () => router.push('/plugins/crm/customers/create?type=legal'),
      icon: <Building className="h-5 w-5" />
    },
    {
      title: intl.formatMessage({
        id: 'crm.quickActions.viewCustomers',
        defaultMessage: 'View All Customers'
      }),
      description: intl.formatMessage({
        id: 'crm.quickActions.viewCustomers.description',
        defaultMessage: 'Browse and manage existing customers'
      }),
      action: () => router.push('/plugins/crm/customers'),
      icon: <Users className="h-5 w-5" />
    },
    {
      title: intl.formatMessage({
        id: 'crm.quickActions.manageLinks',
        defaultMessage: 'Manage Account Links'
      }),
      description: intl.formatMessage({
        id: 'crm.quickActions.manageLinks.description',
        defaultMessage: 'View and manage customer-account relationships'
      }),
      action: () => router.push('/plugins/crm/aliases'),
      icon: <LinkIcon className="h-5 w-5" />
    }
  ]

  return (
    <React.Fragment>
      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'crm.dashboard.title',
              defaultMessage: 'Customer Relationship Management'
            })}
            subtitle={intl.formatMessage({
              id: 'crm.dashboard.subtitle',
              defaultMessage:
                'Manage customer profiles, relationships, and account associations.'
            })}
          />
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'crm.helperTrigger.question',
            defaultMessage: 'What is CRM in Midaz?'
          })}
          answer={intl.formatMessage({
            id: 'crm.helperTrigger.answer',
            defaultMessage:
              'The CRM plugin allows you to manage customer data, create detailed profiles for individuals and companies, and link customers to their ledger accounts for comprehensive relationship management.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/docs/plugins/crm"
        />
      </PageHeader.Root>

      {/* Stats Overview */}
      <div className="mt-8 grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {stats.map((stat) => (
          <Card key={stat.title}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">
                {stat.title}
              </CardTitle>
              {stat.icon}
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{stat.value}</div>
              <div className="flex items-center space-x-1 text-xs text-muted-foreground">
                <TrendingUp className="h-3 w-3" />
                <span className="text-green-600">{stat.change}</span>
                <span>
                  {intl.formatMessage({
                    id: 'crm.stats.fromLastMonth',
                    defaultMessage: 'from last month'
                  })}
                </span>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Quick Actions */}
      <div className="mt-8">
        <h2 className="mb-4 text-lg font-semibold">
          {intl.formatMessage({
            id: 'crm.quickActions.title',
            defaultMessage: 'Quick Actions'
          })}
        </h2>
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          {quickActions.map((action, index) => (
            <Card
              key={index}
              className="cursor-pointer transition-colors hover:bg-muted/50"
              onClick={action.action}
            >
              <CardHeader>
                <div className="flex items-center space-x-2">
                  <div className="rounded-lg bg-primary/10 p-2 text-primary">
                    {action.icon}
                  </div>
                  <CardTitle className="text-sm">{action.title}</CardTitle>
                </div>
              </CardHeader>
              <CardContent>
                <p className="mb-3 text-xs text-muted-foreground">
                  {action.description}
                </p>
                <Button variant="outline" size="sm" className="w-full">
                  {intl.formatMessage({
                    id: 'common.getStarted',
                    defaultMessage: 'Get Started'
                  })}
                  <ArrowRight className="ml-2 h-3 w-3" />
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>

      {/* Recent Activity Placeholder */}
      <div className="mt-8">
        <h2 className="mb-4 text-lg font-semibold">
          {intl.formatMessage({
            id: 'crm.recentActivity.title',
            defaultMessage: 'Recent Activity'
          })}
        </h2>
        <Card>
          <CardContent className="p-6">
            <div className="text-center text-muted-foreground">
              <Users className="mx-auto mb-2 h-12 w-12 opacity-50" />
              <p>
                {intl.formatMessage({
                  id: 'crm.recentActivity.placeholder',
                  defaultMessage:
                    'Customer activity will appear here once you start managing customers.'
                })}
              </p>
            </div>
          </CardContent>
        </Card>
      </div>
    </React.Fragment>
  )
}

export default CRMDashboardPage
