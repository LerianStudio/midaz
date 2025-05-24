'use client'

import React from 'react'
import { useParams } from 'next/navigation'
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
  CreditCard
} from 'lucide-react'
import {
  Customer,
  CustomerType
} from '@/components/crm/customers/customer-types'
import { generateMockCustomers } from '@/components/crm/customers/customer-mock-data'

export default function CustomerDetailPage() {
  const params = useParams()
  const customerId = params.id as string

  // Get customer from mock data
  const customers = generateMockCustomers(50)
  const customer = customers.find((c) => c.id === customerId)

  if (!customer) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center space-y-4">
        <div className="space-y-2 text-center">
          <h2 className="text-xl font-semibold">Customer Not Found</h2>
          <p className="text-muted-foreground">
            The customer you're looking for doesn't exist or has been removed.
          </p>
        </div>
        <Button onClick={() => window.history.back()}>Go Back</Button>
      </div>
    )
  }

  const isNaturalPerson = customer.type === CustomerType.NATURAL_PERSON

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
          <Button variant="outline" size="sm">
            <Edit3 className="mr-2 h-4 w-4" />
            Edit
          </Button>
          <Button variant="outline" size="sm">
            <Trash2 className="mr-2 h-4 w-4" />
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
                    External ID
                  </label>
                  <p className="font-medium">{customer.externalId}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Status
                  </label>
                  <div>
                    <Badge
                      variant={
                        customer.status === 'active' ? 'default' : 'secondary'
                      }
                      className="capitalize"
                    >
                      {customer.status}
                    </Badge>
                  </div>
                </div>
              </div>

              {/* Person-specific information */}
              {isNaturalPerson && customer.naturalPerson && (
                <>
                  <Separator />
                  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Birth Date
                      </label>
                      <p className="font-medium">
                        {new Date(
                          customer.naturalPerson.birthDate
                        ).toLocaleDateString()}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Gender
                      </label>
                      <p className="font-medium">
                        {customer.naturalPerson.gender}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Civil Status
                      </label>
                      <p className="font-medium">
                        {customer.naturalPerson.civilStatus}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Nationality
                      </label>
                      <p className="font-medium">
                        {customer.naturalPerson.nationality}
                      </p>
                    </div>
                  </div>
                </>
              )}

              {/* Company-specific information */}
              {!isNaturalPerson && customer.legalPerson && (
                <>
                  <Separator />
                  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Trade Name
                      </label>
                      <p className="font-medium">
                        {customer.legalPerson.tradeName}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Activity
                      </label>
                      <p className="font-medium">
                        {customer.legalPerson.activity}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Company Type
                      </label>
                      <p className="font-medium">{customer.legalPerson.type}</p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Founded
                      </label>
                      <p className="font-medium">
                        {new Date(
                          customer.legalPerson.foundingDate
                        ).toLocaleDateString()}
                      </p>
                    </div>
                    <div>
                      <label className="text-sm font-medium text-muted-foreground">
                        Company Size
                      </label>
                      <p className="font-medium">{customer.legalPerson.size}</p>
                    </div>
                  </div>

                  {customer.legalPerson.representative && (
                    <>
                      <Separator />
                      <div>
                        <h4 className="mb-2 font-medium">
                          Legal Representative
                        </h4>
                        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                          <div>
                            <label className="text-sm font-medium text-muted-foreground">
                              Name
                            </label>
                            <p className="font-medium">
                              {customer.legalPerson.representative.name}
                            </p>
                          </div>
                          <div>
                            <label className="text-sm font-medium text-muted-foreground">
                              Document
                            </label>
                            <p className="font-medium">
                              {customer.legalPerson.representative.document}
                            </p>
                          </div>
                          <div>
                            <label className="text-sm font-medium text-muted-foreground">
                              Role
                            </label>
                            <p className="font-medium">
                              {customer.legalPerson.representative.role}
                            </p>
                          </div>
                          <div>
                            <label className="text-sm font-medium text-muted-foreground">
                              Email
                            </label>
                            <p className="font-medium">
                              {customer.legalPerson.representative.email}
                            </p>
                          </div>
                        </div>
                      </div>
                    </>
                  )}
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
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="flex items-center space-x-3">
                  <Mail className="h-4 w-4 text-muted-foreground" />
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">
                      Primary Email
                    </label>
                    <p className="font-medium">
                      {customer.contact.primaryEmail}
                    </p>
                  </div>
                </div>
                <div className="flex items-center space-x-3">
                  <Phone className="h-4 w-4 text-muted-foreground" />
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">
                      Mobile Phone
                    </label>
                    <p className="font-medium">
                      {customer.contact.mobilePhone}
                    </p>
                  </div>
                </div>
              </div>

              {customer.contact.secondaryEmail && (
                <div className="flex items-center space-x-3">
                  <Mail className="h-4 w-4 text-muted-foreground" />
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">
                      Secondary Email
                    </label>
                    <p className="font-medium">
                      {customer.contact.secondaryEmail}
                    </p>
                  </div>
                </div>
              )}

              {customer.contact.homePhone && (
                <div className="flex items-center space-x-3">
                  <Phone className="h-4 w-4 text-muted-foreground" />
                  <div>
                    <label className="text-sm font-medium text-muted-foreground">
                      Home Phone
                    </label>
                    <p className="font-medium">{customer.contact.homePhone}</p>
                  </div>
                </div>
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
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Primary Address
                  </label>
                  <div className="mt-1">
                    <p className="font-medium">
                      {customer.addresses.primary.line1}
                    </p>
                    {customer.addresses.primary.line2 && (
                      <p className="text-muted-foreground">
                        {customer.addresses.primary.line2}
                      </p>
                    )}
                    <p className="text-muted-foreground">
                      {customer.addresses.primary.city},{' '}
                      {customer.addresses.primary.state}{' '}
                      {customer.addresses.primary.zipCode}
                    </p>
                    <p className="text-muted-foreground">
                      {customer.addresses.primary.country}
                    </p>
                  </div>
                </div>
              </div>
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
              <Button className="w-full justify-start" variant="outline">
                <CreditCard className="mr-2 h-4 w-4" />
                View Aliases
              </Button>
              <Button className="w-full justify-start" variant="outline">
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
                  Customer Since
                </label>
                <p className="font-medium">
                  {new Date(
                    customer.metadata.customerSince
                  ).toLocaleDateString()}
                </p>
              </div>
              <div>
                <label className="text-sm font-medium text-muted-foreground">
                  Risk Level
                </label>
                <Badge
                  variant={
                    customer.metadata.riskLevel === 'Low'
                      ? 'default'
                      : 'destructive'
                  }
                >
                  {customer.metadata.riskLevel}
                </Badge>
              </div>
              <div>
                <label className="text-sm font-medium text-muted-foreground">
                  Preferred Language
                </label>
                <p className="font-medium">
                  {customer.metadata.preferredLanguage}
                </p>
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
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}
