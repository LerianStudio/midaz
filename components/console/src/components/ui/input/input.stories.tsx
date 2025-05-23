import { Meta, StoryObj } from '@storybook/react'
import { InputProps, Input } from '.'
import { FormField } from '../form'
import { FormProvider, useForm } from 'react-hook-form'

const meta: Meta<InputProps> = {
  title: 'Primitives/Input',
  component: Input,
  argTypes: {
    disabled: {
      type: 'boolean',
      description: 'If the input is disabled'
    },
    className: {
      type: 'string',
      description: "The input's class"
    }
  }
}

export default meta

export const Default: StoryObj<InputProps> = {
  args: {
    placeholder: 'Input'
  },
  render: (args) => {
    const form = useForm()
    return (
      <FormProvider {...form}>
        <Input {...args} />
      </FormProvider>
    )
  }
}
