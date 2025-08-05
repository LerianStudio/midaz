import { Meta, StoryObj } from '@storybook/nextjs'
import { Avatar, AvatarFallback, AvatarImage } from '.'

const meta: Meta = {
  title: 'Primitives/Avatar',
  tags: [''],
  component: Avatar,
  argTypes: {
    children: {
      type: 'string',
      description: "The button's content"
    },
    src: {
      type: 'string',
      description: "The button's content"
    }
  }
}

export default meta

export const AvatarDefault: StoryObj = {
  render: (args) => (
    <Avatar {...args}>
      <AvatarImage src="https://github.com/shadcn.png" />
      <AvatarFallback>CN</AvatarFallback>
    </Avatar>
  )
}

export const AvatarImageDefault: StoryObj = {
  render: (args) => (
    <Avatar {...args}>
      <AvatarImage src="https://github.com/shadcn.png" />
      <AvatarFallback>CN</AvatarFallback>
    </Avatar>
  )
}

export const AvatarFallbackDefault: StoryObj = {
  render: (args) => (
    <Avatar {...args}>
      <AvatarFallback>CN</AvatarFallback>
    </Avatar>
  )
}
