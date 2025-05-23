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

export type OrganizationsFormParentIdFieldProps = {
  name: string
  label?: string
  placeholder?: string
  description?: string
  disabled?: boolean
  control: Control<any>
}

export const OrganizationsFormParentIdField = ({
  label,
  placeholder,
  description,
  ...others
}: OrganizationsFormParentIdFieldProps) => {
  const form = useFormContext<{ id: string }>()
  const { data, isPending } = useListOrganizations({})

  const id = form.watch('id')

  const options = React.useMemo(
    () => data?.items?.filter((org) => org.id !== id),
    [id, data?.items]
  )

  return (
    <FormField
      {...others}
      render={({ field: { ref, onChange, ...fieldOthers } }) => (
        <FormItem ref={ref}>
          {label && <FormLabel>{label}</FormLabel>}
          <FormControl>
            <>
              {isPending && <Skeleton className="h-10 w-full" />}

              {!isPending && (
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
              )}
            </>
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
        </FormItem>
      )}
    />
  )
}
