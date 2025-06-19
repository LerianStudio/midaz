import { Meta, StoryObj } from '@storybook/nextjs'
import { InputField, InputFieldProps } from '.'
import { useForm } from 'react-hook-form'
import { Form } from '@/components/ui/form'

const meta: Meta<InputFieldProps> = {
  title: 'Components/Form/InputField',
  component: InputField,
  argTypes: {}
}

export default meta

function BaseComponent(args: Omit<InputFieldProps, 'name' | 'control'>) {
  const form = useForm()

  return (
    <div className="w-1/2">
      <Form {...form}>
        <InputField
          {...args}
          control={form.control}
          label="Username"
          name="username"
          placeholder="Type..."
        />
      </Form>
    </div>
  )
}

export const Primary: StoryObj<InputFieldProps> = {
  render: (args) => BaseComponent(args)
}

export const Required: StoryObj<InputFieldProps> = {
  args: {
    required: true
  },
  render: (args) => BaseComponent(args)
}

export const WithTooltip: StoryObj<InputFieldProps> = {
  args: {
    tooltip: 'This is a Tooltip!'
  },
  render: (args) => BaseComponent(args)
}

export const WithExtraLabel: StoryObj<InputFieldProps> = {
  args: {
    labelExtra: <span>Extra Label</span>
  },
  render: (args) => BaseComponent(args)
}

export const ReadOnly: StoryObj<InputFieldProps> = {
  args: {
    readOnly: true
  },
  render: (args) => BaseComponent(args)
}

export const Disabled: StoryObj<InputFieldProps> = {
  args: {
    disabled: true
  },
  render: (args) => BaseComponent(args)
}
