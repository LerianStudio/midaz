import React from 'react'
import { Meta, StoryObj } from '@storybook/react'
import { Header, StaticHeader } from '.'

const meta: Meta = {
  title: 'Components/Header',
  component: Header,
  parameters: {
    nextjs: {
      appDirectory: true
    }
  }
}

export default meta

export const Primary: StoryObj = {
  render: (args) => (
    <div className="flex-column flex h-80 bg-zinc-100 p-6">
      <Header {...args} />
    </div>
  )
}

export const StaticHeaderExample: StoryObj = {
  render: (args) => (
    <div className="flex-column flex h-80 bg-zinc-100 p-6">
      <StaticHeader {...args} />
    </div>
  )
}
