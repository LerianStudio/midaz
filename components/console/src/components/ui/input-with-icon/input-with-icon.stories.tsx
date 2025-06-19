import { Meta, StoryObj } from '@storybook/nextjs'
import { InputWithIconProps, InputWithIcon } from '.'
import { Search } from 'lucide-react'
import { FormProvider, useForm } from 'react-hook-form'

const meta: Meta<InputWithIconProps> = {
  title: 'Primitives/InputWithIcon',
  component: InputWithIcon,
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

export const Default: StoryObj<InputWithIconProps> = {
  args: {
    placeholder: 'Input',
    icon: <Search />
  },
  render: (args) => {
    const form = useForm()
    return (
      <FormProvider {...form}>
        <InputWithIcon {...args} />
      </FormProvider>
    )
  }
}
