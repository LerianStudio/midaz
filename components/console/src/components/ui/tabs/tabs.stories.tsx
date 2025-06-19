import { Meta, StoryObj } from '@storybook/nextjs'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '.'
import { TabsProps } from '@radix-ui/react-tabs'

const meta: Meta<TabsProps> = {
  title: 'Primitives/Tabs',
  component: Tabs,
  argTypes: {
    defaultValue: {
      type: 'string',
      defaultValue: 'account'
    },
    value: {
      type: 'string'
    },
    orientation: {
      options: ['horizontal', 'vertical'],
      control: { type: 'radio' },
      defaultValue: 'horizontal'
    }
  }
}

export default meta

type Story = StoryObj<TabsProps>

export const Primary: Story = {
  render: (args) => (
    <Tabs {...args}>
      <TabsList>
        <TabsTrigger value="account">Account</TabsTrigger>
        <TabsTrigger value="password">Password</TabsTrigger>
      </TabsList>
      <TabsContent value="account">Tab Content 1</TabsContent>
      <TabsContent value="password">Tab Content 1</TabsContent>
    </Tabs>
  )
}
