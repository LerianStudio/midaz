'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Users, Puzzle, ArrowRight } from 'lucide-react'
import { useRouter } from 'next/navigation'

const PluginsPage = () => {
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
      })
    }
  ])

  const plugins = [
    {
      id: 'crm',
      name: intl.formatMessage({
        id: 'plugins.crm.name',
        defaultMessage: 'Customer Relationship Management'
      }),
      description: intl.formatMessage({
        id: 'plugins.crm.description',
        defaultMessage:
          'Manage customer data, profiles, and account relationships with comprehensive CRM functionality.'
      }),
      icon: <Users className="h-8 w-8" />,
      available: true,
      href: '/plugins/crm'
    },
    {
      id: 'more',
      name: intl.formatMessage({
        id: 'plugins.more.name',
        defaultMessage: 'More Plugins Coming Soon'
      }),
      description: intl.formatMessage({
        id: 'plugins.more.description',
        defaultMessage:
          'Additional native plugins are being developed to extend Midaz functionality.'
      }),
      icon: <Puzzle className="h-8 w-8" />,
      available: false,
      href: '#'
    }
  ]

  return (
    <React.Fragment>
      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'plugins.title',
              defaultMessage: 'Native Plugins'
            })}
            subtitle={intl.formatMessage({
              id: 'plugins.subtitle',
              defaultMessage:
                'Extend Midaz with powerful native plugins for enhanced functionality.'
            })}
          />
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'plugins.helperTrigger.question',
            defaultMessage: 'What are Native Plugins?'
          })}
          answer={intl.formatMessage({
            id: 'plugins.helperTrigger.answer',
            defaultMessage:
              'Native plugins are first-party extensions that integrate seamlessly with Midaz to provide specialized functionality like customer management, analytics, and more.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/docs/plugins"
        />
      </PageHeader.Root>

      <div className="mt-10">
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {plugins.map((plugin) => (
            <Card key={plugin.id} className="p-6">
              <div className="flex items-start space-x-4">
                <div
                  className={`rounded-lg p-2 ${plugin.available ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'}`}
                >
                  {plugin.icon}
                </div>
                <div className="flex-1 space-y-2">
                  <h3 className="text-lg font-semibold">{plugin.name}</h3>
                  <p className="text-sm text-muted-foreground">
                    {plugin.description}
                  </p>
                  <div className="pt-4">
                    <Button
                      variant={plugin.available ? 'default' : 'secondary'}
                      size="sm"
                      disabled={!plugin.available}
                      onClick={() =>
                        plugin.available && router.push(plugin.href)
                      }
                      className="w-full"
                    >
                      {plugin.available ? (
                        <>
                          {intl.formatMessage({
                            id: 'plugins.open',
                            defaultMessage: 'Open Plugin'
                          })}
                          <ArrowRight className="ml-2 h-4 w-4" />
                        </>
                      ) : (
                        intl.formatMessage({
                          id: 'plugins.comingSoon',
                          defaultMessage: 'Coming Soon'
                        })
                      )}
                    </Button>
                  </div>
                </div>
              </div>
            </Card>
          ))}
        </div>
      </div>
    </React.Fragment>
  )
}

export default PluginsPage
