'use client'

import React from 'react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { CustomerType } from './customer-types'

// Create safe input component that doesn't use form context
const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => {
    return (
      <input
        type={type}
        className={`flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 md:text-sm ${className || ''}`}
        ref={ref}
        {...props}
      />
    )
  }
)
Input.displayName = 'Input'

// Create safe label component that doesn't use form context
const Label = React.forwardRef<HTMLLabelElement, React.LabelHTMLAttributes<HTMLLabelElement>>(
  ({ className, ...props }, ref) => {
    return (
      <label
        ref={ref}
        className={`text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70 ${className || ''}`}
        {...props}
      />
    )
  }
)
Label.displayName = 'Label'

interface CustomerWizardProps {
  step: number
  customerType: CustomerType
  formData: any
  onDataUpdate: (data: any) => void
}

export const CustomerWizard: React.FC<CustomerWizardProps> = ({
  step,
  customerType,
  formData,
  onDataUpdate
}) => {
  const isNaturalPerson = customerType === CustomerType.NATURAL_PERSON

  const handleInputChange = (field: string, value: string) => {
    onDataUpdate({ ...formData, [field]: value })
  }

  // Step 2: Basic Information Form
  const BasicInfoForm = () => {
    return (
      <div className="space-y-6">
        <div>
          <h3 className="mb-2 text-lg font-medium">Basic Information</h3>
          <p className="mb-6 text-muted-foreground">
            {isNaturalPerson
              ? "Enter the individual customer's personal information."
              : "Enter the company's business information."}
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="name">
              {isNaturalPerson ? 'Full Name' : 'Company Name'}
            </Label>
            <Input
              id="name"
              placeholder={isNaturalPerson ? 'John Doe' : 'Acme Corp'}
              value={formData.name || ''}
              onChange={(e) => handleInputChange('name', e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="document">
              {isNaturalPerson ? 'CPF' : 'CNPJ'}
            </Label>
            <Input
              id="document"
              placeholder={
                isNaturalPerson ? '000.000.000-00' : '00.000.000/0000-00'
              }
              value={formData.document || ''}
              onChange={(e) => handleInputChange('document', e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="externalId">External ID (Optional)</Label>
            <Input
              id="externalId"
              placeholder="CUST_2024_001"
              value={formData.externalId || ''}
              onChange={(e) => handleInputChange('externalId', e.target.value)}
            />
          </div>
        </div>

        {/* Natural Person specific fields */}
        {isNaturalPerson && (
          <div className="border-t pt-6">
            <h4 className="mb-4 font-medium">Personal Details</h4>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="birthDate">Birth Date</Label>
                <Input
                  id="birthDate"
                  type="date"
                  value={formData.birthDate || ''}
                  onChange={(e) => handleInputChange('birthDate', e.target.value)}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="gender">Gender</Label>
                <Select onValueChange={(value) => handleInputChange('gender', value)}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select gender" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="Male">Male</SelectItem>
                    <SelectItem value="Female">Female</SelectItem>
                    <SelectItem value="Other">Other</SelectItem>
                    <SelectItem value="Prefer not to say">Prefer not to say</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label htmlFor="civilStatus">Civil Status</Label>
                <Select onValueChange={(value) => handleInputChange('civilStatus', value)}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select status" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="Single">Single</SelectItem>
                    <SelectItem value="Married">Married</SelectItem>
                    <SelectItem value="Divorced">Divorced</SelectItem>
                    <SelectItem value="Widowed">Widowed</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label htmlFor="nationality">Nationality</Label>
                <Input
                  id="nationality"
                  placeholder="Brazilian"
                  value={formData.nationality || ''}
                  onChange={(e) => handleInputChange('nationality', e.target.value)}
                />
              </div>
            </div>
          </div>
        )}

        {/* Legal Person specific fields */}
        {!isNaturalPerson && (
          <>
            <div className="border-t pt-6">
              <h4 className="mb-4 font-medium">Company Details</h4>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="tradeName">Trade Name</Label>
                  <Input
                    id="tradeName"
                    placeholder="Acme"
                    value={formData.tradeName || ''}
                    onChange={(e) => handleInputChange('tradeName', e.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="activity">Business Activity</Label>
                  <Input
                    id="activity"
                    placeholder="Software Development"
                    value={formData.activity || ''}
                    onChange={(e) => handleInputChange('activity', e.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="companyType">Company Type</Label>
                  <Select onValueChange={(value) => handleInputChange('companyType', value)}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select type" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="Limited Liability Company">
                        Limited Liability Company
                      </SelectItem>
                      <SelectItem value="Corporation">Corporation</SelectItem>
                      <SelectItem value="Partnership">Partnership</SelectItem>
                      <SelectItem value="Sole Proprietorship">
                        Sole Proprietorship
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="foundingDate">Founding Date</Label>
                  <Input
                    id="foundingDate"
                    type="date"
                    value={formData.foundingDate || ''}
                    onChange={(e) => handleInputChange('foundingDate', e.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="companySize">Company Size</Label>
                  <Select onValueChange={(value) => handleInputChange('companySize', value)}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select size" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="Micro">Micro (1-9 employees)</SelectItem>
                      <SelectItem value="Small">Small (10-49 employees)</SelectItem>
                      <SelectItem value="Medium">Medium (50-249 employees)</SelectItem>
                      <SelectItem value="Large">Large (250+ employees)</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </div>

            <div className="border-t pt-6">
              <h4 className="mb-4 font-medium">Legal Representative</h4>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="representativeName">Representative Name</Label>
                  <Input
                    id="representativeName"
                    placeholder="John Doe"
                    value={formData.representativeName || ''}
                    onChange={(e) => handleInputChange('representativeName', e.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="representativeDocument">Representative CPF</Label>
                  <Input
                    id="representativeDocument"
                    placeholder="000.000.000-00"
                    value={formData.representativeDocument || ''}
                    onChange={(e) => handleInputChange('representativeDocument', e.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="representativeRole">Role</Label>
                  <Input
                    id="representativeRole"
                    placeholder="CEO"
                    value={formData.representativeRole || ''}
                    onChange={(e) => handleInputChange('representativeRole', e.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="representativeEmail">Representative Email</Label>
                  <Input
                    id="representativeEmail"
                    placeholder="john@company.com"
                    value={formData.representativeEmail || ''}
                    onChange={(e) => handleInputChange('representativeEmail', e.target.value)}
                  />
                </div>
              </div>
            </div>
          </>
        )}
      </div>
    )
  }

  // Step 3: Contact Information Form
  const ContactForm = () => {
    return (
      <div className="space-y-6">
        <div>
          <h3 className="mb-2 text-lg font-medium">Contact Information</h3>
          <p className="mb-6 text-muted-foreground">
            Enter contact details for communication.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="primaryEmail">Primary Email *</Label>
            <Input
              id="primaryEmail"
              type="email"
              placeholder="john@example.com"
              value={formData.primaryEmail || ''}
              onChange={(e) => handleInputChange('primaryEmail', e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="secondaryEmail">Secondary Email</Label>
            <Input
              id="secondaryEmail"
              type="email"
              placeholder="john.personal@example.com"
              value={formData.secondaryEmail || ''}
              onChange={(e) => handleInputChange('secondaryEmail', e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="mobilePhone">Mobile Phone *</Label>
            <Input
              id="mobilePhone"
              placeholder="+55 11 99999-9999"
              value={formData.mobilePhone || ''}
              onChange={(e) => handleInputChange('mobilePhone', e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="homePhone">Home Phone</Label>
            <Input
              id="homePhone"
              placeholder="+55 11 3333-3333"
              value={formData.homePhone || ''}
              onChange={(e) => handleInputChange('homePhone', e.target.value)}
            />
          </div>
        </div>
      </div>
    )
  }

  // Step 4: Address Information Form
  const AddressForm = () => {
    return (
      <div className="space-y-6">
        <div>
          <h3 className="mb-2 text-lg font-medium">Address Information</h3>
          <p className="mb-6 text-muted-foreground">
            Enter the primary address for this customer.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4">
          <div className="space-y-2">
            <Label htmlFor="line1">Address Line 1 *</Label>
            <Input
              id="line1"
              placeholder="123 Main Street"
              value={formData.line1 || ''}
              onChange={(e) => handleInputChange('line1', e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="line2">Address Line 2</Label>
            <Input
              id="line2"
              placeholder="Apt 4B, Building A"
              value={formData.line2 || ''}
              onChange={(e) => handleInputChange('line2', e.target.value)}
            />
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <div className="space-y-2">
              <Label htmlFor="city">City *</Label>
              <Input
                id="city"
                placeholder="São Paulo"
                value={formData.city || ''}
                onChange={(e) => handleInputChange('city', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="state">State *</Label>
              <Input
                id="state"
                placeholder="SP"
                value={formData.state || ''}
                onChange={(e) => handleInputChange('state', e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="zipCode">ZIP Code *</Label>
              <Input
                id="zipCode"
                placeholder="01234-567"
                value={formData.zipCode || ''}
                onChange={(e) => handleInputChange('zipCode', e.target.value)}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="country">Country *</Label>
            <Select onValueChange={(value) => handleInputChange('country', value)}>
              <SelectTrigger>
                <SelectValue placeholder="Select country" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="BR">Brazil</SelectItem>
                <SelectItem value="US">United States</SelectItem>
                <SelectItem value="CA">Canada</SelectItem>
                <SelectItem value="MX">Mexico</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>
    )
  }

  // Step 5: Review Form
  const ReviewForm = () => {
    return (
      <div className="space-y-6">
        <div>
          <h3 className="mb-2 text-lg font-medium">Review Information</h3>
          <p className="mb-6 text-muted-foreground">
            Please review all information before creating the customer.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          {/* Basic Information */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Basic Information</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div>
                <span className="text-sm text-muted-foreground">Type:</span>
                <Badge className="ml-2">
                  {isNaturalPerson ? 'Individual' : 'Corporate'}
                </Badge>
              </div>
              <div>
                <span className="text-sm text-muted-foreground">Name:</span>
                <span className="ml-2 font-medium">{formData.name || 'Not provided'}</span>
              </div>
              <div>
                <span className="text-sm text-muted-foreground">Document:</span>
                <span className="ml-2 font-medium">{formData.document || 'Not provided'}</span>
              </div>
              {formData.externalId && (
                <div>
                  <span className="text-sm text-muted-foreground">External ID:</span>
                  <span className="ml-2 font-medium">{formData.externalId}</span>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Contact Information */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Contact Information</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div>
                <span className="text-sm text-muted-foreground">Primary Email:</span>
                <span className="ml-2 font-medium">{formData.primaryEmail || 'Not provided'}</span>
              </div>
              <div>
                <span className="text-sm text-muted-foreground">Mobile Phone:</span>
                <span className="ml-2 font-medium">{formData.mobilePhone || 'Not provided'}</span>
              </div>
              {formData.secondaryEmail && (
                <div>
                  <span className="text-sm text-muted-foreground">Secondary Email:</span>
                  <span className="ml-2 font-medium">{formData.secondaryEmail}</span>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Address Information */}
          <Card className="lg:col-span-2">
            <CardHeader>
              <CardTitle className="text-base">Address Information</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-1">
                <p className="font-medium">{formData.line1 || 'Address not provided'}</p>
                {formData.line2 && <p className="text-muted-foreground">{formData.line2}</p>}
                <p className="text-muted-foreground">
                  {formData.city || 'City'}, {formData.state || 'State'} {formData.zipCode || 'ZIP'}
                </p>
                <p className="text-muted-foreground">{formData.country || 'Country'}</p>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  // Render appropriate form based on step
  switch (step) {
    case 2:
      return <BasicInfoForm />
    case 3:
      return <ContactForm />
    case 4:
      return <AddressForm />
    case 5:
      return <ReviewForm />
    default:
      return null
  }
}