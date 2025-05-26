import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormTooltip
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { ReactNode } from 'react'
import { Control } from 'react-hook-form'

export type SwitchFieldProps = {
  label?: string
  name: string
  control: Control<any>
  labelExtra?: ReactNode
  tooltip?: string
  required?: boolean
  disabled?: boolean
  disabledTooltip?: string
}

export const SwitchField = ({
  label,
  name,
  control,
  labelExtra,
  tooltip,
  required,
  disabled,
  disabledTooltip
}: SwitchFieldProps) => {
  return (
    <FormField
      name={name}
      control={control}
      render={({ field }) => (
        <FormItem required={required}>
          {label && (
            <FormLabel
              extra={
                tooltip ? <FormTooltip>{tooltip}</FormTooltip> : labelExtra
              }
            >
              {label}
            </FormLabel>
          )}

          <div className="relative">
            <FormControl>
              {disabled && disabledTooltip ? (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <div className="inline-flex w-auto">
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                          disabled={disabled}
                        />
                      </div>
                    </TooltipTrigger>
                    <TooltipContent>{disabledTooltip}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              ) : (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  disabled={disabled}
                />
              )}
            </FormControl>
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
