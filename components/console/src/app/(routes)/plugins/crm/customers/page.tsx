'use client'

import React, { useState } from 'react'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  UserPlus,
  Building,
  Search,
  Filter,
  Users,
  Mail,
  Phone,
  MapPin,
  MoreHorizontal
} from 'lucide-react'
import { useRouter } from 'next/navigation'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import {
  generateMockCustomers,
  generateMockAliases
} from '@/components/crm/customers/customer-mock-data'

const CustomersPage = () => {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization } = useOrganization()
  const [searchTerm, setSearchTerm] = useState('')

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
      }),
      href: '/plugins/crm'
    },
    {
      name: intl.formatMessage({
        id: 'crm.customers',
        defaultMessage: 'Customers'
      })
    }
  ])

  // Generate mock customer data
  const mockCustomers = generateMockCustomers(50)
  const allAliases = generateMockAliases(200)

  // Enhance customers with alias counts
  const enrichedCustomers = mockCustomers.map((customer) => ({
    ...customer,
    aliasCount: allAliases.filter((alias) => alias.holderId === customer.id)
      .length
  }))

  const filteredCustomers = enrichedCustomers.filter(
    (customer) =>
      customer.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      customer.document.includes(searchTerm) ||
      customer.contact.primaryEmail
        .toLowerCase()
        .includes(searchTerm.toLowerCase())
  )

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'Active':
        return 'bg-green-100 text-green-800'
      case 'Pending':
        return 'bg-yellow-100 text-yellow-800'
      case 'Inactive':
        return 'bg-gray-100 text-gray-800'
      default:
        return 'bg-gray-100 text-gray-800'
    }
  }

  const getCustomerTypeIcon = (type: string) => {
    return type === 'NATURAL_PERSON' ? (
      <Users className="h-4 w-4" />
    ) : (
      <Building className="h-4 w-4" />
    )
  }

  const getCustomerTypeLabel = (type: string) => {
    return type === 'NATURAL_PERSON'
      ? intl.formatMessage({
          id: 'crm.customerType.individual',
          defaultMessage: 'Individual'
        })
      : intl.formatMessage({
          id: 'crm.customerType.corporate',
          defaultMessage: 'Corporate'
        })
  }

  return (
    <React.Fragment>
      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'crm.customers.title',
              defaultMessage: 'Customers'
            })}
            subtitle={intl.formatMessage({
              id: 'crm.customers.subtitle',
              defaultMessage:
                'Manage individual and corporate customer profiles and their account relationships.'
            })}
          />
          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'crm.customers.helperTrigger.question',
                defaultMessage: 'What are Customers?'
              })}
            />

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button>
                  <UserPlus className="mr-2 h-4 w-4" />
                  {intl.formatMessage({
                    id: 'crm.customers.newCustomer',
                    defaultMessage: 'New Customer'
                  })}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem
                  onClick={() =>
                    router.push('/plugins/crm/customers/create?type=natural')
                  }
                >
                  <Users className="mr-2 h-4 w-4" />
                  {intl.formatMessage({
                    id: 'crm.customers.newIndividual',
                    defaultMessage: 'Individual Customer'
                  })}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() =>
                    router.push('/plugins/crm/customers/create?type=legal')
                  }
                >
                  <Building className="mr-2 h-4 w-4" />
                  {intl.formatMessage({
                    id: 'crm.customers.newCorporate',
                    defaultMessage: 'Corporate Customer'
                  })}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'crm.customers.helperTrigger.question',
            defaultMessage: 'What are Customers?'
          })}
          answer={intl.formatMessage({
            id: 'crm.customers.helperTrigger.answer',
            defaultMessage:
              'Customers represent individuals or companies that use your financial services. Each customer profile contains personal information, contact details, and can be linked to multiple ledger accounts through aliases.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/docs/plugins/crm/customers"
        />
      </PageHeader.Root>

      {/* Search and Filters */}
      <div className="mt-8 flex items-center space-x-4">
        <div className="relative max-w-sm flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={intl.formatMessage({
              id: 'crm.customers.search.placeholder',
              defaultMessage: 'Search customers by name, document, or email...'
            })}
            value={searchTerm}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
              setSearchTerm(e.target.value)
            }
            className="pl-10"
          />
        </div>
        <Button variant="outline">
          <Filter className="mr-2 h-4 w-4" />
          {intl.formatMessage({
            id: 'common.filters',
            defaultMessage: 'Filters'
          })}
        </Button>
      </div>

      {/* Customer List */}
      <div className="mt-6 space-y-4">
        {filteredCustomers.length === 0 ? (
          <Card className="p-8">
            <div className="text-center">
              <Users className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
              <h3 className="mb-2 text-lg font-semibold">
                {intl.formatMessage({
                  id: 'crm.customers.empty.title',
                  defaultMessage: 'No customers found'
                })}
              </h3>
              <p className="mb-4 text-muted-foreground">
                {searchTerm
                  ? intl.formatMessage({
                      id: 'crm.customers.empty.search',
                      defaultMessage: 'No customers match your search criteria.'
                    })
                  : intl.formatMessage({
                      id: 'crm.customers.empty.description',
                      defaultMessage:
                        'Get started by creating your first customer profile.'
                    })}
              </p>
              {!searchTerm && (
                <Button
                  onClick={() => router.push('/plugins/crm/customers/create')}
                >
                  <UserPlus className="mr-2 h-4 w-4" />
                  {intl.formatMessage({
                    id: 'crm.customers.createFirst',
                    defaultMessage: 'Create Your First Customer'
                  })}
                </Button>
              )}
            </div>
          </Card>
        ) : (
          filteredCustomers.map((customer) => (
            <Card
              key={customer.id}
              className="cursor-pointer p-6 transition-shadow hover:shadow-md"
              onClick={() =>
                router.push(`/plugins/crm/customers/${customer.id}`)
              }
            >
              <div className="flex items-start justify-between">
                <div className="flex items-start space-x-4">
                  <div className="rounded-lg bg-primary/10 p-2 text-primary">
                    {getCustomerTypeIcon(customer.type)}
                  </div>
                  <div className="space-y-2">
                    <div className="flex items-center space-x-2">
                      <h3 className="font-semibold">{customer.name}</h3>
                      <Badge className={getStatusColor(customer.status)}>
                        {customer.status}
                      </Badge>
                      <Badge variant="outline">
                        {getCustomerTypeLabel(customer.type)}
                      </Badge>
                    </div>
                    <div className="space-y-1 text-sm text-muted-foreground">
                      <div className="flex items-center space-x-4">
                        <span>{customer.document}</span>
                        <div className="flex items-center space-x-1">
                          <Mail className="h-3 w-3" />
                          <span>{customer.contact.primaryEmail}</span>
                        </div>
                        <div className="flex items-center space-x-1">
                          <Phone className="h-3 w-3" />
                          <span>{customer.contact.mobilePhone}</span>
                        </div>
                      </div>
                      <div className="flex items-center space-x-1">
                        <MapPin className="h-3 w-3" />
                        <span>
                          {customer.addresses.primary.city},{' '}
                          {customer.addresses.primary.state},{' '}
                          {customer.addresses.primary.country}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
                <div className="flex items-center space-x-4">
                  <div className="text-right text-sm">
                    <div className="font-medium">
                      {customer.aliasCount}{' '}
                      {intl.formatMessage({
                        id: 'crm.customers.aliases',
                        defaultMessage: 'aliases'
                      })}
                    </div>
                    <div className="text-muted-foreground">
                      {intl.formatMessage({
                        id: 'crm.customers.created',
                        defaultMessage: 'Created'
                      })}{' '}
                      {new Date(customer.createdAt).toLocaleDateString()}
                    </div>
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={(e: React.MouseEvent) => e.stopPropagation()}
                      >
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onClick={(e: React.MouseEvent) => {
                          e.stopPropagation()
                          router.push(`/plugins/crm/customers/${customer.id}`)
                        }}
                      >
                        {intl.formatMessage({
                          id: 'common.view',
                          defaultMessage: 'View'
                        })}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e: React.MouseEvent) => {
                          e.stopPropagation()
                          router.push(
                            `/plugins/crm/customers/${customer.id}/edit`
                          )
                        }}
                      >
                        {intl.formatMessage({
                          id: 'common.edit',
                          defaultMessage: 'Edit'
                        })}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e: React.MouseEvent) => {
                          e.stopPropagation()
                          router.push(
                            `/plugins/crm/customers/${customer.id}/aliases`
                          )
                        }}
                      >
                        {intl.formatMessage({
                          id: 'crm.customers.manageAliases',
                          defaultMessage: 'Manage Aliases'
                        })}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
            </Card>
          ))
        )}
      </div>
    </React.Fragment>
  )
}

export default CustomersPage
