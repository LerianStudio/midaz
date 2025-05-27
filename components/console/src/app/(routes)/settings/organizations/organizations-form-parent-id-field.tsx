import { useListOrganizations } from '@/client/organizations'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import React from 'react'
import { Control, useFormContext } from 'react-hook-form'
import type { OrganizationFormData } from './organizations-form'

export type OrganizationsFormParentIdFieldProps = {
  name: keyof OrganizationFormData
  label?: string
  placeholder?: string
  description?: string
  disabled?: boolean
  readOnly?: boolean
  control: Control<OrganizationFormData>
  value?: string | null
  onChange?: (value: string | null) => void
}

export const OrganizationsFormParentIdField = ({
  label,
  placeholder,
  description,
  readOnly,
  value,
  onChange,
  ...others
}: OrganizationsFormParentIdFieldProps) => {
  const form = useFormContext<OrganizationFormData>()
  const { data, isPending } = useListOrganizations({})

  const id = form.watch('id')

  const options = React.useMemo(
    () => data?.items?.filter((org) => org.id !== id),
    [id, data?.items]
  )

  return (
    <FormField
      {...others}
      render={({ field: { value, onChange, disabled } }) => (
        <FormItem>
          {label && <FormLabel>{label}</FormLabel>}
          <FormControl>
            <React.Fragment>
              {isPending && <Skeleton className="h-10 w-full" />}
              {!isPending && (
                <Select 
                  value={(value as string) || undefined} 
                  onValueChange={onChange}
                  disabled={disabled}
                >
                  <SelectTrigger readOnly={readOnly}>
                    <SelectValue placeholder={placeholder} />
                  </SelectTrigger>
                  <SelectContent>
                    {options?.map((parent) => (
                      <SelectItem key={parent.id} value={parent.id!}>
                        {parent.legalName}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </React.Fragment>
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
        </FormItem>
      )}
    />
  )
}
