import { Meta, StoryObj } from '@storybook/nextjs'
import { Label } from '.'

const meta: Meta = {
  title: 'Primitives/Label',
  component: Label,
  argTypes: {
    className: {
      type: 'string',
      description: "The label's class"
    },
    children: {
      type: 'string',
      description: "The label's text"
    }
  }
}

export default meta

export const Default: StoryObj = {
  args: {
    children: 'Label'
  }
}
