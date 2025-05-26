import { Meta, StoryObj } from '@storybook/react'
import { Button } from '../ui/button'
import React from 'react'
import { EmptyResource, EmptyResourceProps } from '.'
import { Plus } from 'lucide-react'

const meta: Meta<EmptyResourceProps> = {
  title: 'Components/EmptyResource',
  component: EmptyResource,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<EmptyResourceProps> = {
  render: (args) => (
    <EmptyResource message="You haven't created any Ledger yet" {...args} />
  )
}

export const WithButton: StoryObj<EmptyResourceProps> = {
  render: (args) => (
    <EmptyResource message="You haven't created any Ledger yet" {...args}>
      <Button variant="outline" icon={<Plus />}>
        Create
      </Button>
    </EmptyResource>
  )
}

export const WithExtra: StoryObj<EmptyResourceProps> = {
  render: (args) => (
    <EmptyResource
      message="You haven't created any Ledger yet"
      extra="No Ledger found."
      {...args}
    >
      <Button variant="outline" icon={<Plus />}>
        Create
      </Button>
    </EmptyResource>
  )
}
