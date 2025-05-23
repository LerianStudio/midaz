import { Meta, StoryObj } from '@storybook/react'
import { Avatar, AvatarFallback, AvatarImage, AvatarProps } from '.'

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
  args: {
    children: 'AvatarImage',
    src: 'https://github.com/shadcn.png'
  }
}

export const AvatarImageDefault: StoryObj = {
  args: {
    children: 'AvatarImage',

    src: 'https://github.com/shadcn.png'
  }
}

export const AvatarFallbackDefault: StoryObj = {
  args: {
    children: 'CN'
  }
}
