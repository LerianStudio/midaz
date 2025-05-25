'use client'

import React, { useState, useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  ArrowLeft,
  Save,
  Loader2,
  Users,
  Building2,
  FileText,
  Phone,
  MapPin
} from 'lucide-react'
import { useHolderById, useUpdateHolder } from '@/client/holders'
import { useToast } from '@/hooks/use-toast'
import { UpdateHolderEntity } from '@/core/domain/entities/holder-entity'

export default function EditCustomerPage() {
  const params = useParams()
  const router = useRouter()
  const { toast } = useToast()
  const customerId = params.id as string

  // Fetch customer data
  const { data: customer, isLoading } = useHolderById({
    holderId: customerId,
    enabled: !!customerId
  })

  // Form state
  const [formData, setFormData] = useState<UpdateHolderEntity>({})
  const [isSaving, setIsSaving] = useState(false)

  // Update mutation
  const updateHolderMutation = useUpdateHolder({
    onSuccess: () => {
      toast({
        title: 'Customer updated successfully',
        description: 'The changes have been saved.'
      })
      router.push(`/plugins/crm/customers/${customerId}`)
    },
    onError: (error) => {
      toast({
        title: 'Failed to update customer',
        description: error.message || 'Please try again.',
        variant: 'destructive'
      })
      setIsSaving(false)
    }
  })

  // Initialize form with customer data
  useEffect(() => {
    if (customer) {
      setFormData({
        name: customer.name,
        status: customer.status,
        address: customer.address,
        tradingName: customer.tradingName,
        legalName: customer.legalName,
        website: customer.website,
        establishedOn: customer.establishedOn,
        monthlyIncomeTotal: customer.monthlyIncomeTotal,
        contacts: customer.contacts,
        metadata: customer.metadata
      })
    }
  }, [customer])

  const handleInputChange = (field: keyof UpdateHolderEntity, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }))
  }

  const handleAddressChange = (field: string, value: string) => {
    setFormData((prev) => ({
      ...prev,
      address: {
        ...prev.address,
        [field]: value
      } as any
    }))
  }

  const handleContactChange = (
    index: number,
    field: 'name' | 'value',
    value: string
  ) => {
    const newContacts = [...(formData.contacts || [])]
    newContacts[index] = { ...newContacts[index], [field]: value }
    setFormData((prev) => ({ ...prev, contacts: newContacts }))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSaving(true)
    updateHolderMutation.mutate({ holderId: customerId, data: formData })
  }

  if (isLoading || !customer) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const isNaturalPerson = customer.type === 'NATURAL_PERSON'

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-4">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.push(`/plugins/crm/customers/${customerId}`)}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-2xl font-bold">Edit Customer</h1>
            <p className="text-muted-foreground">
              Update {customer.name}&apos;s information
            </p>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <Badge variant="outline">
            {isNaturalPerson ? 'Natural Person' : 'Legal Person'}
          </Badge>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
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
              <div className="space-y-2">
                <Label htmlFor="name">
                  {isNaturalPerson ? 'Full Name' : 'Company Name'}
                </Label>
                <Input
                  id="name"
                  value={formData.name || ''}
                  onChange={(e) => handleInputChange('name', e.target.value)}
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="status">Status</Label>
                <Select
                  value={formData.status || ''}
                  onValueChange={(value) => handleInputChange('status', value)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select status" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="Active">Active</SelectItem>
                    <SelectItem value="Inactive">Inactive</SelectItem>
                    <SelectItem value="Pending">Pending</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {/* Company specific fields */}
              {!isNaturalPerson && (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="tradingName">Trade Name</Label>
                    <Input
                      id="tradingName"
                      value={formData.tradingName || ''}
                      onChange={(e) =>
                        handleInputChange('tradingName', e.target.value)
                      }
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="legalName">Legal Name</Label>
                    <Input
                      id="legalName"
                      value={formData.legalName || ''}
                      onChange={(e) =>
                        handleInputChange('legalName', e.target.value)
                      }
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="website">Website</Label>
                    <Input
                      id="website"
                      type="url"
                      value={formData.website || ''}
                      onChange={(e) =>
                        handleInputChange('website', e.target.value)
                      }
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="establishedOn">Established On</Label>
                    <Input
                      id="establishedOn"
                      type="date"
                      value={formData.establishedOn || ''}
                      onChange={(e) =>
                        handleInputChange('establishedOn', e.target.value)
                      }
                    />
                  </div>
                </>
              )}

              {/* Natural person specific fields */}
              {isNaturalPerson && (
                <div className="space-y-2">
                  <Label htmlFor="monthlyIncomeTotal">Monthly Income</Label>
                  <Input
                    id="monthlyIncomeTotal"
                    type="number"
                    value={formData.monthlyIncomeTotal || ''}
                    onChange={(e) =>
                      handleInputChange(
                        'monthlyIncomeTotal',
                        Number(e.target.value)
                      )
                    }
                  />
                </div>
              )}
            </div>
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
            {formData.contacts?.map((contact, index) => (
              <div
                key={index}
                className="grid grid-cols-1 gap-4 md:grid-cols-2"
              >
                <div className="space-y-2">
                  <Label>Contact Type</Label>
                  <Select
                    value={contact.name}
                    onValueChange={(value) =>
                      handleContactChange(index, 'name', value)
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="email">Email</SelectItem>
                      <SelectItem value="phone">Phone</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>Value</Label>
                  <Input
                    value={contact.value}
                    onChange={(e) =>
                      handleContactChange(index, 'value', e.target.value)
                    }
                  />
                </div>
              </div>
            ))}
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
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 gap-4">
              <div className="space-y-2">
                <Label htmlFor="line1">Address Line 1</Label>
                <Input
                  id="line1"
                  value={formData.address?.line1 || ''}
                  onChange={(e) => handleAddressChange('line1', e.target.value)}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="line2">Address Line 2</Label>
                <Input
                  id="line2"
                  value={formData.address?.line2 || ''}
                  onChange={(e) => handleAddressChange('line2', e.target.value)}
                />
              </div>

              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <div className="space-y-2">
                  <Label htmlFor="city">City</Label>
                  <Input
                    id="city"
                    value={formData.address?.city || ''}
                    onChange={(e) =>
                      handleAddressChange('city', e.target.value)
                    }
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="state">State</Label>
                  <Input
                    id="state"
                    value={formData.address?.state || ''}
                    onChange={(e) =>
                      handleAddressChange('state', e.target.value)
                    }
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="zipCode">Zip Code</Label>
                  <Input
                    id="zipCode"
                    value={formData.address?.zipCode || ''}
                    onChange={(e) =>
                      handleAddressChange('zipCode', e.target.value)
                    }
                  />
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="country">Country</Label>
                <Input
                  id="country"
                  value={formData.address?.country || ''}
                  onChange={(e) =>
                    handleAddressChange('country', e.target.value)
                  }
                />
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Actions */}
        <div className="flex items-center justify-between">
          <Button
            type="button"
            variant="outline"
            onClick={() => router.push(`/plugins/crm/customers/${customerId}`)}
          >
            Cancel
          </Button>

          <Button type="submit" disabled={isSaving}>
            {isSaving ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              <>
                <Save className="mr-2 h-4 w-4" />
                Save Changes
              </>
            )}
          </Button>
        </div>
      </form>
    </div>
  )
}
