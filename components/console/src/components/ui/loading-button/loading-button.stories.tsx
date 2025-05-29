import { Meta, StoryObj } from '@storybook/react'
import { LoadingButton, LoadingButtonProps } from '.'
import { Users } from 'lucide-react'
import React from 'react'

const meta: Meta<LoadingButtonProps> = {
  title: 'Primitives/LoadingButton',
  component: LoadingButton,
  argTypes: {
    children: {
      type: 'string',
      description: "The button's content"
    },
    disabled: {
      type: 'boolean',
      description: 'If the button is disabled'
    },
    className: {
      type: 'string',
      description: "The button's class"
    }
  }
}

export default meta

export const Primary: StoryObj<LoadingButtonProps> = {
  args: {
    loading: true,
    children: 'Button'
  }
}

export const Secundary: StoryObj<LoadingButtonProps> = {
  args: {
    loading: true,
    children: 'Button',
    variant: 'secondary'
  }
}

export const Outline: StoryObj<LoadingButtonProps> = {
  args: {
    loading: true,
    children: 'Button',
    variant: 'outline'
  }
}

export const WithIcon: StoryObj<LoadingButtonProps> = {
  args: {
    loading: true,
    children: 'Button'
  },
  render: (args) => <LoadingButton icon={<Users />} {...args} />
}

export const StateChange: StoryObj<LoadingButtonProps> = {
  args: {
    children: 'Save'
  },
  render: (args) => {
    const [loading, setLoading] = React.useState(false)

    const handleClick = () => {
      setLoading(true)
      setTimeout(() => {
        setLoading(false)
      }, 2000)
    }

    return <LoadingButton loading={loading} onClick={handleClick} {...args} />
  }
}
