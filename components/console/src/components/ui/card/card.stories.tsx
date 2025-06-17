import { Meta, StoryObj } from '@storybook/nextjs'
import {
  Card,
  CardTitle,
  CardHeader,
  CardDescription,
  CardContent,
  CardFooter
} from '.'
import { Button } from '../button'

const meta: Meta<typeof Card> = {
  title: 'Primitives/Card',
  component: Card,
  argTypes: {
    className: {
      type: 'string',
      description: 'The card class'
    }
  }
}

export default meta

export const Primary: StoryObj<typeof Card> = {
  render: (args) => (
    <Card className="w-[350px]" {...args}>
      <CardHeader>
        <CardTitle>Create project</CardTitle>
        <CardDescription>Deploy your new project in one-click.</CardDescription>
      </CardHeader>
      <CardContent>This is the Card Content</CardContent>
      <CardFooter className="flex justify-between">
        <Button variant="outline">Cancel</Button>
        <Button>Deploy</Button>
      </CardFooter>
    </Card>
  )
}
