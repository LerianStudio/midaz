import { Meta, StoryObj } from '@storybook/react'
import { SelectField, SelectFieldProps } from '.'
import { useForm } from 'react-hook-form'
import { Form } from '@/components/ui/form'
import { SelectItem } from '@/components/ui/select'

const meta: Meta<SelectFieldProps> = {
  title: 'Components/Form/SelectField',
  component: SelectField,
  argTypes: {}
}

export default meta

function BaseComponent(args: Omit<SelectFieldProps, 'name' | 'control'>) {
  const form = useForm()

  return (
    <div className="w-1/2">
      <Form {...form}>
        <SelectField
          {...args}
          control={form.control}
          label="Fruits"
          name="fruits"
          placeholder="Select..."
        >
          <SelectItem value="apple">Apple</SelectItem>
          <SelectItem value="banana">Banana</SelectItem>
          <SelectItem value="orange">Orange</SelectItem>
        </SelectField>
      </Form>
    </div>
  )
}

export const Primary: StoryObj<SelectFieldProps> = {
  render: (args) => BaseComponent(args)
}

export const Required: StoryObj<SelectFieldProps> = {
  args: {
    required: true
  },
  render: (args) => BaseComponent(args)
}

export const WithTooltip: StoryObj<SelectFieldProps> = {
  args: {
    tooltip: 'This is a Tooltip!'
  },
  render: (args) => BaseComponent(args)
}

export const WithExtraLabel: StoryObj<SelectFieldProps> = {
  args: {
    labelExtra: <span>Extra Label</span>
  },
  render: (args) => BaseComponent(args)
}

export const Disabled: StoryObj<SelectFieldProps> = {
  args: {
    disabled: true
  },
  render: (args) => BaseComponent(args)
}
