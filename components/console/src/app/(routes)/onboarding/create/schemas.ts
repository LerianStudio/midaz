import { z } from 'zod'
import { organization } from '@/schema/organization'

export const detailFormSchema = z.object({
  legalName: organization.legalName,
  doingBusinessAs: organization.doingBusinessAs,
  legalDocument: organization.legalDocument
})
export type DetailFormData = z.infer<typeof detailFormSchema>

export const addressFormSchema = z.object({
  address: z.object({
    line1: organization.address.line1,
    line2: organization.address.line2,
    city: organization.address.city,
    state: organization.address.state,
    country: organization.address.country,
    zipCode: organization.address.zipCode
  })
})
export type AddressFormData = z.infer<typeof addressFormSchema>

export const themeFormSchema = z.object({
  accentColor: organization.accentColor,
  avatar: organization.avatar
})
export type ThemeFormData = z.infer<typeof themeFormSchema>

export type FormData = DetailFormData & AddressFormData & ThemeFormData
