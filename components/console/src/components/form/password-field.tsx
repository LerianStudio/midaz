import { useState } from 'react'
import { Control, FieldPath, FieldValues } from 'react-hook-form'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormTooltip
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { EyeIcon, EyeOffIcon } from 'lucide-react'

interface PasswordFieldProps<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>
> {
  name: TName
  label: string
  tooltip?: string
  control: Control<TFieldValues>
  required?: boolean
  disabled?: boolean
}

export function PasswordField<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>
>({
  name,
  label,
  tooltip,
  control,
  required = false,
  disabled = false
}: PasswordFieldProps<TFieldValues, TName>) {
  const [showPassword, setShowPassword] = useState(false)

  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem required={required}>
          <FormLabel
            extra={tooltip ? <FormTooltip>{tooltip}</FormTooltip> : undefined}
          >
            {label}
          </FormLabel>
          <div className="relative">
            <FormControl>
              <Input
                {...field}
                type={showPassword ? 'text' : 'password'}
                disabled={disabled}
                className="pr-10"
              />
            </FormControl>
            <button
              type="button"
              className="absolute inset-y-0 right-0 flex items-center px-3 text-gray-400 hover:text-gray-500 focus:outline-none"
              tabIndex={-1}
              onMouseDown={(e) => {
                e.preventDefault()
                setShowPassword(!showPassword)
              }}
            >
              {showPassword ? <EyeOffIcon size={20} /> : <EyeIcon size={20} />}
            </button>
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
