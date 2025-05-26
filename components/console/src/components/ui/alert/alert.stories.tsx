import { Meta, StoryObj } from '@storybook/react'
import { Alert, AlertDescription, AlertProps, AlertTitle } from '.'
import { Terminal } from 'lucide-react'

const meta: Meta<AlertProps> = {
  title: 'Primitives/Alert',
  component: Alert,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<AlertProps> = {
  render: (args) => (
    <Alert {...args}>
      <Terminal className="h-4 w-4" />
      <AlertTitle>Heads up!</AlertTitle>
      <AlertDescription>This is an alert component</AlertDescription>
    </Alert>
  )
}

export const Destructive: StoryObj<AlertProps> = {
  args: {
    variant: 'destructive'
  },
  render: (args) => (
    <Alert {...args}>
      <Terminal className="h-4 w-4" />
      <AlertTitle>Error</AlertTitle>
      <AlertDescription>
        Your session has expired. Please log in again.
      </AlertDescription>
    </Alert>
  )
}
