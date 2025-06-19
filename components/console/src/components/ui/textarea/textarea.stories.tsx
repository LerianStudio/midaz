import { ComponentProps } from 'react'
import { Meta, StoryObj } from '@storybook/nextjs'
import { Textarea } from '.'
import { FormProvider, useForm } from 'react-hook-form'

const meta: Meta<ComponentProps<'textarea'>> = {
  title: 'Primitives/Textarea',
  component: Textarea,
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

export const Default: StoryObj<ComponentProps<'textarea'>> = {
  args: {
    placeholder: 'Textarea...'
  },
  render: (args) => {
    const form = useForm()
    return (
      <FormProvider {...form}>
        <Textarea {...args} />
      </FormProvider>
    )
  }
}
