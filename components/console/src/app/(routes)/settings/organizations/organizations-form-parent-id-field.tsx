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
import { Input } from '@/components/ui/input'

export type OrganizationsFormParentIdFieldProps = {
  name: string
  label?: string
  placeholder?: string
  description?: string
  disabled?: boolean
  readOnly?: boolean
  control: Control<any>
}

export const OrganizationsFormParentIdField = ({
  label,
  placeholder,
  description,
  readOnly,
  ...others
}: OrganizationsFormParentIdFieldProps) => {
  const form = useFormContext<{ id: string }>()
  const { data, isPending } = useListOrganizations({})

  const id = form.watch('id')

  const options = React.useMemo(
    () => data?.items?.filter((org) => org.id !== id),
    [id, data?.items]
  )

  const getOrganizationName = React.useCallback(
    (orgId: string) => {
      return options?.find((org) => org.id === orgId)?.legalName || orgId
    },
    [options]
  )

  return (
    <FormField
      {...others}
      render={({ field: { ref, onChange, ...fieldOthers } }) => (
        <FormItem ref={ref}>
          {label && <FormLabel>{label}</FormLabel>}
          <FormControl>
            <React.Fragment>
              {isPending && <Skeleton className="h-10 w-full" />}

              {!isPending && readOnly ? (
                <Input
                  value={
                    fieldOthers.value
                      ? getOrganizationName(fieldOthers.value)
                      : placeholder || ''
                  }
                  readOnly={true}
                />
              ) : (
                !isPending && (
                  <Select onValueChange={onChange} {...fieldOthers}>
                    <SelectTrigger>
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
                )
              )}
            </React.Fragment>
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
        </FormItem>
      )}
    />
  )
}
