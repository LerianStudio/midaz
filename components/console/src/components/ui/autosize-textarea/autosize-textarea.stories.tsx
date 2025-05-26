import { ComponentProps } from 'react'
import { Meta, StoryObj } from '@storybook/react'
import { AutosizeTextarea, AutosizeTextAreaProps } from '.'
import { FormProvider, useForm } from 'react-hook-form'

const meta: Meta<AutosizeTextAreaProps> = {
  title: 'Primitives/AutosizeTextarea',
  component: AutosizeTextarea,
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

export const Default: StoryObj<AutosizeTextAreaProps> = {
  args: {
    placeholder: 'This textarea with min height 52 and unlimited max height.'
  },
  render: (args) => {
    const form = useForm()
    return (
      <FormProvider {...form}>
        <AutosizeTextarea {...args} />
      </FormProvider>
    )
  }
}

export const MaxHeight: StoryObj<AutosizeTextAreaProps> = {
  args: {
    placeholder: 'This textarea with min height 52 and max height 200.',
    maxHeight: 200
  },
  render: (args) => {
    const form = useForm()
    return (
      <FormProvider {...form}>
        <AutosizeTextarea {...args} />
      </FormProvider>
    )
  }
}
