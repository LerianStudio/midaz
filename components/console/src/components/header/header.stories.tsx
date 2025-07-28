import React from 'react'
import { Meta, StoryObj } from '@storybook/nextjs'
import { StaticHeader } from '.'

const meta: Meta = {
  title: 'Components/Header',
  component: StaticHeader,
  parameters: {
    nextjs: {
      appDirectory: true
    }
  }
}

export default meta

export const StaticHeaderExample: StoryObj = {
  render: (args) => (
    <div className="flex-column flex h-80 bg-zinc-100 p-6">
      <StaticHeader {...args} />
    </div>
  )
}
