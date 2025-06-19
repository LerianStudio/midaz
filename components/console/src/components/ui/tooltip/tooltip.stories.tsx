import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '.'
import { Meta, StoryObj } from '@storybook/nextjs'
import { Button } from '../button'
import { TooltipProps } from '@radix-ui/react-tooltip'

const meta: Meta<TooltipProps> = {
  title: 'Primitives/Tooltip',
  component: Tooltip,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<TooltipProps> = {
  render: (args) => (
    <TooltipProvider>
      <Tooltip {...args}>
        <TooltipTrigger asChild>
          <Button className="mt-6" variant="outline">
            Hover
          </Button>
        </TooltipTrigger>
        <TooltipContent>Add to library</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export const WithoutDelay: StoryObj<TooltipProps> = {
  args: {
    delayDuration: 0
  },
  render: (args) => (
    <TooltipProvider>
      <Tooltip {...args}>
        <TooltipTrigger asChild>
          <Button className="mt-6" variant="outline">
            Hover
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>Add to library</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export const WithSides: StoryObj<TooltipProps> = {
  render: (args) => (
    <div className="m-6 flex flex-row gap-12">
      <TooltipProvider>
        <Tooltip {...args}>
          <TooltipTrigger asChild>
            <Button variant="outline">Top</Button>
          </TooltipTrigger>
          <TooltipContent side="top">
            <p>This is a Tooltip!</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <TooltipProvider>
        <Tooltip {...args}>
          <TooltipTrigger asChild>
            <Button variant="outline">Bottom</Button>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            <p>This is a Tooltip!</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <TooltipProvider>
        <Tooltip {...args}>
          <TooltipTrigger asChild>
            <Button variant="outline">Left</Button>
          </TooltipTrigger>
          <TooltipContent side="left">
            <p>This is a Tooltip!</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <TooltipProvider>
        <Tooltip {...args}>
          <TooltipTrigger asChild>
            <Button variant="outline">Right</Button>
          </TooltipTrigger>
          <TooltipContent side="right">
            <p>This is a Tooltip!</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  )
}
