import React from 'react'
import { HashIcon } from 'lucide-react'
import { Control, ControllerRenderProps } from 'react-hook-form'
import { ChromePicker, ColorResult } from 'react-color'
import { FormDescription, FormField, FormItem } from '@/components/ui/form'
import { InputWithIcon } from '@/components/ui/input-with-icon'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@/components/ui/popover'

type ColorInputProps = Omit<ControllerRenderProps, 'ref'>

const ColorInput = React.forwardRef<HTMLInputElement, ColorInputProps>(
  ({ name, value, onChange, ...others }: ColorInputProps, ref) => {
    const [open, setOpen] = React.useState(false)

    const handleInputChange = (event: any) => {
      onChange?.({
        ...event,
        target: { ...event.target, value: `#${event.target.value}` }
      })
    }

    const handleChange = (color: ColorResult) => {
      onChange?.({ target: { name, value: color.hex } })
    }

    return (
      <div className="mb-4 flex w-full flex-col gap-2">
        <Popover open={open} onOpenChange={setOpen}>
          <div className="flex w-full gap-2">
            <PopoverTrigger asChild>
              <div
                className="h-9 w-9 flex-shrink-0 cursor-pointer rounded-md border border-zinc-300 hover:border-zinc-400"
                style={{
                  backgroundColor: value !== '' ? value : '#FFFFFF'
                }}
              />
            </PopoverTrigger>

            <InputWithIcon
              icon={<HashIcon />}
              value={value?.replace('#', '')}
              onChange={handleInputChange}
              disabled
              {...others}
            />
          </div>

          <PopoverContent className="w-auto p-0" side="bottom" align="start">
            <ChromePicker
              color={value}
              disableAlpha
              onChange={handleChange}
              onChangeComplete={handleChange}
              styles={{
                default: {
                  picker: {
                    boxShadow:
                      '0px 10px 15px -3px rgba(0, 0, 0, 0.10), 0px 4px 6px -2px rgba(0, 0, 0, 0.05)'
                  }
                }
              }}
            />
          </PopoverContent>
        </Popover>
      </div>
    )
  }
)
ColorInput.displayName = 'ColorInput'

export type OrganizationsFormColorFieldProps = {
  name: string
  description?: string
  control: Control<any>
}

export const OrganizationsFormColorField = ({
  description,
  ...others
}: OrganizationsFormColorFieldProps) => {
  return (
    <FormField
      {...others}
      render={({ field }) => (
        <FormItem>
          <ColorInput {...field} />
          <FormDescription>{description}</FormDescription>
        </FormItem>
      )}
    />
  )
}
