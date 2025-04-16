import { Meta, StoryObj } from '@storybook/react'
import { Separator } from '.'

const meta: Meta<React.HTMLAttributes<HTMLDivElement>> = {
  title: 'Primitives/Separator',
  component: Separator,
  argTypes: {}
}

export default meta

export const Primary: StoryObj = {
  render: () => (
    <div>
      <div className="space-y-1">
        <h4 className="text-sm font-medium leading-none">Radix Primitives</h4>
        <p className="text-sm text-muted-foreground">
          An open-source UI component library.
        </p>
      </div>
      <Separator className="my-4" />
      <div className="flex h-5 items-center space-x-4 text-sm">
        <div>Blog</div>
        <Separator orientation="vertical" />
        <div>Docs</div>
        <Separator orientation="vertical" />
        <div>Source</div>
      </div>
    </div>
  )
}
