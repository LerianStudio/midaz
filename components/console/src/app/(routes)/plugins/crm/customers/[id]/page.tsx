'use client'

import React from 'react'
import { useParams, useRouter } from 'next/navigation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Users,
  Building2,
  Mail,
  Phone,
  MapPin,
  Calendar,
  FileText,
  Edit3,
  Trash2,
  Plus,
  CreditCard,
  Loader2,
  AlertCircle
} from 'lucide-react'
import { useHolderById, useDeleteHolder } from '@/client/holders'
import { useListAliases } from '@/client/aliases'
import { useToast } from '@/hooks/use-toast'
import { Alert, AlertDescription } from '@/components/ui/alert'

export default function CustomerDetailPage() {
  const params = useParams()
  const router = useRouter()
  const { toast } = useToast()
  const customerId = params.id as string

  // Fetch customer data
  const {
    data: customer,
    isLoading,
    error
  } = useHolderById({
    holderId: customerId,
    enabled: !!customerId
  })

  // Fetch aliases
  const { data: aliasesData } = useListAliases({
    holderId: customerId,
    page: 1,
    limit: 10,
    enabled: !!customerId
  })

  // Delete holder mutation
  const deleteHolderMutation = useDeleteHolder({
    onSuccess: () => {
      toast({
        title: 'Customer deleted successfully',
        description: 'The customer has been removed from your CRM.'
      })
      router.push('/plugins/crm/customers')
    },
    onError: (error) => {
      toast({
        title: 'Failed to delete customer',
        description: error.message || 'Please try again.',
        variant: 'destructive'
      })
    }
  })

  const handleDelete = () => {
    if (confirm('Are you sure you want to delete this customer?')) {
      deleteHolderMutation.mutate({ holderId: customerId })
    }
  }

  if (isLoading) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !customer) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center space-y-4">
        <div className="space-y-2 text-center">
          <AlertCircle className="mx-auto h-12 w-12 text-destructive" />
          <h2 className="text-xl font-semibold">Customer Not Found</h2>
          <p className="text-muted-foreground">
            The customer you&apos;re looking for doesn&apos;t exist or has been
            removed.
          </p>
        </div>
        <Button onClick={() => router.push('/plugins/crm/customers')}>
          Go Back
        </Button>
      </div>
    )
  }

  const isNaturalPerson = customer.type === 'NATURAL_PERSON'
  const aliasCount = aliasesData?.items?.length || 0

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-4">
          <div className="flex items-center space-x-2">
            {isNaturalPerson ? (
              <Users className="h-6 w-6 text-blue-600" />
            ) : (
              <Building2 className="h-6 w-6 text-purple-600" />
            )}
            <div>
              <h1 className="text-2xl font-bold">{customer.name}</h1>
              <p className="text-muted-foreground">
                {isNaturalPerson ? 'Individual Customer' : 'Corporate Customer'}
              </p>
            </div>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              router.push(`/plugins/crm/customers/${customerId}/edit`)
            }
          >
            <Edit3 className="mr-2 h-4 w-4" />
            Edit
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleDelete}
            disabled={deleteHolderMutation.isPending}
          >
            {deleteHolderMutation.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Trash2 className="mr-2 h-4 w-4" />
            )}
            Delete
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Main Information */}
        <div className="space-y-6 lg:col-span-2">
          {/* Basic Information */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center space-x-2">
                <FileText className="h-5 w-5" />
                <span>Basic Information</span>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Full Name
                  </label>
                  <p className="font-medium">{customer.name}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Document
                  </label>
                  <p className="font-medium">{customer.document}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Customer Type
                  </label>
                  <p className="font-medium">
                    {isNaturalPerson ? 'Natural Person' : 'Legal Person'}
                  </p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Status
                  </label>
                  <div>
                    <Badge
                      variant={
                        customer.status === 'Active' ? 'default' : 'secondary'
                      }
                      className="capitalize"
                    >
                      {customer.status}
                    </Badge>
                  </div>
                </div>
              </div>

              {/* Person-specific information */}
              {isNaturalPerson && (
                <>
                  <Separator />
                  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                    {customer.monthlyIncomeTotal && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">
                          Monthly Income
                        </label>
                        <p className="font-medium">
                          ${customer.monthlyIncomeTotal.toLocaleString()}
                        </p>
                      </div>
                    )}
                    {customer.metadata?.birthDate && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">
                          Birth Date
                        </label>
                        <p className="font-medium">
                          {new Date(
                            customer.metadata.birthDate
                          ).toLocaleDateString()}
                        </p>
                      </div>
                    )}
                  </div>
                </>
              )}

              {/* Company-specific information */}
              {!isNaturalPerson && (
                <>
                  <Separator />
                  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                    {customer.tradingName && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">
                          Trade Name
                        </label>
                        <p className="font-medium">{customer.tradingName}</p>
                      </div>
                    )}
                    {customer.legalName && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">
                          Legal Name
                        </label>
                        <p className="font-medium">{customer.legalName}</p>
                      </div>
                    )}
                    {customer.website && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">
                          Website
                        </label>
                        <p className="font-medium">{customer.website}</p>
                      </div>
                    )}
                    {customer.establishedOn && (
                      <div>
                        <label className="text-sm font-medium text-muted-foreground">
                          Established On
                        </label>
                        <p className="font-medium">
                          {new Date(
                            customer.establishedOn
                          ).toLocaleDateString()}
                        </p>
                      </div>
                    )}
                  </div>
                </>
              )}
            </CardContent>
          </Card>

          {/* Contact Information */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center space-x-2">
                <Phone className="h-5 w-5" />
                <span>Contact Information</span>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {customer.contacts && customer.contacts.length > 0 ? (
                <div className="space-y-4">
                  {customer.contacts.map((contact, index) => (
                    <div key={index} className="flex items-center space-x-3">
                      {contact.name === 'email' ? (
                        <Mail className="h-4 w-4 text-muted-foreground" />
                      ) : (
                        <Phone className="h-4 w-4 text-muted-foreground" />
                      )}
                      <div>
                        <label className="text-sm font-medium capitalize text-muted-foreground">
                          {contact.name}
                        </label>
                        <p className="font-medium">{contact.value}</p>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-muted-foreground">
                  No contact information available
                </p>
              )}
            </CardContent>
          </Card>

          {/* Address Information */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center space-x-2">
                <MapPin className="h-5 w-5" />
                <span>Address Information</span>
              </CardTitle>
            </CardHeader>
            <CardContent>
              {customer.address ? (
                <div className="space-y-4">
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">
                      Address
                    </label>
                    <div className="mt-1">
                      <p className="font-medium">{customer.address.line1}</p>
                      {customer.address.line2 && (
                        <p className="text-muted-foreground">
                          {customer.address.line2}
                        </p>
                      )}
                      <p className="text-muted-foreground">
                        {customer.address.city}, {customer.address.state}{' '}
                        {customer.address.zipCode}
                      </p>
                      <p className="text-muted-foreground">
                        {customer.address.country}
                      </p>
                    </div>
                  </div>
                </div>
              ) : (
                <p className="text-muted-foreground">
                  No address information available
                </p>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Sidebar */}
        <div className="space-y-6">
          {/* Quick Actions */}
          <Card>
            <CardHeader>
              <CardTitle>Quick Actions</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <Button
                className="w-full justify-start"
                variant="outline"
                onClick={() =>
                  router.push(`/plugins/crm/customers/${customerId}/aliases`)
                }
              >
                <CreditCard className="mr-2 h-4 w-4" />
                View Aliases ({aliasCount})
              </Button>
              <Button
                className="w-full justify-start"
                variant="outline"
                onClick={() =>
                  router.push(
                    `/plugins/crm/customers/${customerId}/aliases/create`
                  )
                }
              >
                <Plus className="mr-2 h-4 w-4" />
                Create Alias
              </Button>
              <Button className="w-full justify-start" variant="outline">
                <FileText className="mr-2 h-4 w-4" />
                Generate Report
              </Button>
            </CardContent>
          </Card>

          {/* Metadata */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center space-x-2">
                <Calendar className="h-5 w-5" />
                <span>Metadata</span>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <label className="text-sm font-medium text-muted-foreground">
                  Customer ID
                </label>
                <p className="font-mono text-xs">{customer.id}</p>
              </div>
              <div>
                <label className="text-sm font-medium text-muted-foreground">
                  Created
                </label>
                <p className="font-medium">
                  {new Date(customer.createdAt).toLocaleDateString()}
                </p>
              </div>
              <div>
                <label className="text-sm font-medium text-muted-foreground">
                  Last Updated
                </label>
                <p className="font-medium">
                  {new Date(customer.updatedAt).toLocaleDateString()}
                </p>
              </div>
              {customer.metadata &&
                Object.keys(customer.metadata).length > 0 && (
                  <>
                    <Separator />
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Custom Metadata
                      </label>
                      <div className="mt-2 space-y-1">
                        {Object.entries(customer.metadata).map(
                          ([key, value]) => (
                            <div key={key} className="text-sm">
                              <span className="font-medium">{key}:</span>{' '}
                              <span className="text-muted-foreground">
                                {String(value)}
                              </span>
                            </div>
                          )
                        )}
                      </div>
                    </div>
                  </>
                )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}
