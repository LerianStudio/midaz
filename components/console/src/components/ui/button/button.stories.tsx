import { Meta, StoryObj } from '@storybook/nextjs'
import { ButtonProps, Button } from '.'
import { Users } from 'lucide-react'

const meta: Meta<ButtonProps> = {
  title: 'Primitives/Button',
  component: Button,
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

export const Primary: StoryObj<ButtonProps> = {
  args: {
    children: 'Button'
  }
}

export const Disabled: StoryObj<ButtonProps> = {
  args: {
    children: 'Button',
    disabled: true
  }
}

export const Secundary: StoryObj<ButtonProps> = {
  args: {
    children: 'Button',
    variant: 'secondary'
  }
}

export const SecundaryDisabled: StoryObj<ButtonProps> = {
  args: {
    children: 'Button',
    variant: 'secondary',
    disabled: true
  }
}

export const Outline: StoryObj<ButtonProps> = {
  args: {
    children: 'Button',
    variant: 'outline'
  }
}

export const FullWidth: StoryObj<ButtonProps> = {
  args: {
    fullWidth: true,
    children: 'Button'
  },
  render: (args) => <Button {...args} />
}

export const WithIcon: StoryObj<ButtonProps> = {
  args: {
    children: 'Button'
  },
  render: (args) => (
    <div className="flex flex-col gap-4">
      <div className="flex flex-row gap-4">
        <Button icon={<Users />} {...args} />
        <Button icon={<Users />} {...args} />
      </div>
      <div className="flex flex-row gap-4">
        <Button fullWidth icon={<Users />} {...args} />
        <Button fullWidth iconPlacement="far-end" icon={<Users />} {...args} />
      </div>
    </div>
  )
}
