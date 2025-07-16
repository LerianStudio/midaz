import { FormItem, FormLabel } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { ReactNode } from 'react'

export type ReadOnlyFieldProps = {
  label: ReactNode
  value: string
}

export const ReadOnlyField = ({ label, value }: ReadOnlyFieldProps) => {
  return (
    <FormItem>
      <FormLabel>{label}</FormLabel>
      <Input readOnly value={value} />
    </FormItem>
  )
} 