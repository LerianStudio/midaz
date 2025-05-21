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
import { Check, Copy } from 'lucide-react'
import { useToast } from '@/hooks/use-toast'
import { useIntl } from 'react-intl'

interface CopyableInputFieldProps<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>
> {
  name: TName
  label: string
  tooltip?: string
  control: Control<TFieldValues>
  readOnly?: boolean
  required?: boolean
  disabled?: boolean
}

export function CopyableInputField<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>
>({
  name,
  label,
  tooltip,
  control,
  readOnly = false,
  required = false,
  disabled = false
}: CopyableInputFieldProps<TFieldValues, TName>) {
  const [isCopied, setIsCopied] = useState(false)
  const { toast } = useToast()
  const intl = useIntl()

  const handleCopy = (value: string) => {
    navigator.clipboard.writeText(value).then(() => {
      setIsCopied(true)

      toast({
        description: intl.formatMessage({
          id: 'common.copied',
          defaultMessage: 'Copied to clipboard'
        })
      })

      setTimeout(() => {
        setIsCopied(false)
      }, 2000)
    })
  }

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
                disabled={disabled}
                readOnly={readOnly}
                className="pr-10"
              />
            </FormControl>
            <button
              type="button"
              className="absolute inset-y-0 right-0 flex items-center px-3 text-gray-400 hover:text-gray-500 focus:outline-none"
              tabIndex={-1}
              onClick={() => handleCopy(field.value)}
            >
              {isCopied ? <Check size={20} /> : <Copy size={20} />}
            </button>
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
