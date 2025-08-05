import * as React from 'react'
import * as LabelPrimitive from '@radix-ui/react-label'
import { Slot } from '@radix-ui/react-slot'
import {
  Controller,
  ControllerProps,
  FieldPath,
  FieldValues,
  FormProvider,
  useFormContext
} from 'react-hook-form'

import { cn } from '@/lib/utils'
import { Label } from '@/components/ui/label'

import { TooltipProviderProps } from '@radix-ui/react-tooltip'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from './tooltip'
import { HelpCircle } from 'lucide-react'

const Form = FormProvider

type FormFieldContextValue<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>
> = {
  name: TName
}

const FormFieldContext = React.createContext<FormFieldContextValue>(
  {} as FormFieldContextValue
)

const FormField = <
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>
>({
  ...props
}: ControllerProps<TFieldValues, TName>) => {
  return (
    <FormFieldContext.Provider value={{ name: props.name }}>
      <Controller {...props} />
    </FormFieldContext.Provider>
  )
}

const useFormField = () => {
  const fieldContext = React.useContext(FormFieldContext)
  const itemContext = React.useContext(FormItemContext)
  const { getFieldState, formState } = useFormContext()

  const fieldState = getFieldState(fieldContext.name, formState)

  if (!fieldContext) {
    throw new Error('useFormField should be used within <FormField>')
  }

  const { id, required } = itemContext

  return {
    id,
    name: fieldContext.name,
    formItemId: `${id}-form-item`,
    formDescriptionId: `${id}-form-item-description`,
    formMessageId: `${id}-form-item-message`,
    required,
    ...fieldState
  }
}

type FormItemContextValue = {
  id: string
  required?: boolean
}

const FormItemContext = React.createContext<FormItemContextValue>(
  {} as FormItemContextValue
)

export type FormItemProps = React.HTMLAttributes<HTMLDivElement> & {
  required?: boolean
}

const FormItem = React.forwardRef<HTMLDivElement, FormItemProps>(
  ({ className, required, ...props }, ref) => {
    const id = React.useId()

    return (
      <FormItemContext.Provider value={{ id, required }}>
        <div ref={ref} className={cn('space-y-2', className)} {...props} />
      </FormItemContext.Provider>
    )
  }
)
FormItem.displayName = 'FormItem'

export type FormLabelProps = React.ComponentPropsWithoutRef<
  typeof LabelPrimitive.Root
> & {
  extra?: React.ReactNode
}

const FormLabel = React.forwardRef<
  React.ElementRef<typeof LabelPrimitive.Root>,
  FormLabelProps
>(({ className, extra, children, ...props }, ref) => {
  const { required, error, formItemId } = useFormField()

  return (
    <Label
      ref={ref}
      className={cn(
        'flex justify-between text-sm font-semibold text-[#52525b]',
        error && 'text-destructive',
        className
      )}
      htmlFor={formItemId}
      {...props}
    >
      <span>
        {children}
        {required ? ' *' : ''}
      </span>
      {extra}
    </Label>
  )
})
FormLabel.displayName = 'FormLabel'

export const FormTooltip = ({ children, ...others }: TooltipProviderProps) => (
  <TooltipProvider {...others}>
    <Tooltip>
      <TooltipTrigger asChild>
        <HelpCircle className="ml-2 h-4 w-4 text-gray-400" />
      </TooltipTrigger>
      <TooltipContent>{children}</TooltipContent>
    </Tooltip>
  </TooltipProvider>
)

const FormControl = React.forwardRef<
  React.ElementRef<typeof Slot>,
  React.ComponentPropsWithoutRef<typeof Slot>
>(({ ...props }, ref) => {
  const { error, formItemId, formDescriptionId, formMessageId } = useFormField()

  return (
    <Slot
      ref={ref}
      id={formItemId}
      aria-describedby={
        !error
          ? `${formDescriptionId}`
          : `${formDescriptionId} ${formMessageId}`
      }
      aria-invalid={!!error}
      {...props}
    />
  )
})
FormControl.displayName = 'FormControl'

const FormDescription = React.forwardRef<
  HTMLParagraphElement,
  React.HTMLAttributes<HTMLParagraphElement>
>(({ className, ...props }, ref) => {
  const { formDescriptionId } = useFormField()

  return (
    <p
      ref={ref}
      id={formDescriptionId}
      className={cn('text-shadcn-400 text-xs font-medium', className)}
      {...props}
    />
  )
})
FormDescription.displayName = 'FormDescription'

const FormMessage = React.forwardRef<
  HTMLParagraphElement,
  React.HTMLAttributes<HTMLParagraphElement>
>(({ className, children, ...props }, ref) => {
  const { error, formMessageId } = useFormField()
  const body = error ? String(error?.message) : children

  if (!body) {
    return null
  }

  return (
    <p
      ref={ref}
      id={formMessageId}
      className={cn('text-destructive text-sm font-medium', className)}
      {...props}
    >
      {body}
    </p>
  )
})
FormMessage.displayName = 'FormMessage'

export {
  useFormField,
  Form,
  FormItem,
  FormLabel,
  FormControl,
  FormDescription,
  FormMessage,
  FormField
}
