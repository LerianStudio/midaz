import { Meta, StoryObj } from '@storybook/nextjs'
import { SwitchField, SwitchFieldProps } from '.'
import { useForm } from 'react-hook-form'
import { Form } from '@/components/ui/form'

const meta: Meta<SwitchFieldProps> = {
  title: 'Components/Form/SwitchField',
  component: SwitchField,
  argTypes: {}
}

export default meta

function BaseComponent(args: Omit<SwitchFieldProps, 'name' | 'control'>) {
  const form = useForm()

  return (
    <div className="w-1/4">
      <Form {...form}>
        <SwitchField
          {...args}
          control={form.control}
          label="Fruits"
          name="fruits"
        />
      </Form>
    </div>
  )
}

export const Primary: StoryObj<SwitchFieldProps> = {
  render: (args) => BaseComponent(args)
}

export const Required: StoryObj<SwitchFieldProps> = {
  args: {
    required: true
  },
  render: (args) => BaseComponent(args)
}

export const WithTooltip: StoryObj<SwitchFieldProps> = {
  args: {
    tooltip: 'This is a Tooltip!'
  },
  render: (args) => BaseComponent(args)
}

export const WithExtraLabel: StoryObj<SwitchFieldProps> = {
  args: {
    labelExtra: <span>Extra Label</span>
  },
  render: (args) => BaseComponent(args)
}

export const Disabled: StoryObj<SwitchFieldProps> = {
  args: {
    disabled: true
  },
  render: (args) => BaseComponent(args)
}
