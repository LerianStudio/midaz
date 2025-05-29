import { Popover, PopoverContent, PopoverTrigger } from '.'
import { Meta, StoryObj } from '@storybook/nextjs'
import { Button } from '../button'
import { PopoverProps } from '@radix-ui/react-popover'

const meta: Meta<PopoverProps> = {
  title: 'Primitives/Popover',
  component: Popover,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<PopoverProps> = {
  render: (args) => (
    <Popover {...args}>
      <PopoverTrigger asChild>
        <Button variant="outline">Open popover</Button>
      </PopoverTrigger>
      <PopoverContent className="w-80">
        <p>This is a Popover!</p>
      </PopoverContent>
    </Popover>
  )
}
