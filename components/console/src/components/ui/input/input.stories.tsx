import { Meta, StoryObj } from '@storybook/nextjs'
import { Input } from '.'
import { FormProvider, useForm } from 'react-hook-form'

const meta: Meta<typeof Input> = {
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

export const Default: StoryObj<typeof Input> = {
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
