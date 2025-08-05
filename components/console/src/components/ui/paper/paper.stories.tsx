import { Meta, StoryObj } from '@storybook/nextjs'
import { Paper, PaperProps } from '.'

const meta: Meta<PaperProps> = {
  title: 'Primitives/Paper',
  component: Paper,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<PaperProps> = {
  args: {
    className: 'p-4'
  },
  render: (args) => (
    <Paper {...args}>
      <p>This is a Paper component!</p>
      <p>
        Lorem ipsum dolor sit amet, consectetur adipiscing elit. Morbi neque
        dolor, tempus ac scelerisque sed, accumsan eget orci. Donec ac mauris
        congue, mollis massa vel, sagittis libero.
      </p>
    </Paper>
  )
}
