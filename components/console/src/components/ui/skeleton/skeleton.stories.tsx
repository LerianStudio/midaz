import { Meta, StoryObj } from '@storybook/nextjs'
import { Skeleton } from '.'

const meta: Meta<React.HTMLAttributes<HTMLDivElement>> = {
  title: 'Primitives/Skeleton',
  component: Skeleton,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<React.HTMLAttributes<HTMLDivElement>> = {
  render: (args) => <Skeleton {...args} className="h-4 w-full" />
}

export const Example: StoryObj<React.HTMLAttributes<HTMLDivElement>> = {
  render: (args) => (
    <div className="flex w-full flex-row gap-4">
      <Skeleton {...args} className="h-16 w-16 rounded-full" />
      <div className="flex flex-grow flex-col gap-2">
        <Skeleton {...args} className="h-4" />
        <Skeleton {...args} className="h-4" />
        <Skeleton {...args} className="h-4" />
      </div>
    </div>
  )
}
