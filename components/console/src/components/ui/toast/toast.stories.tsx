import { Meta, StoryObj } from '@storybook/nextjs'
import { Toaster } from './toaster'
import {
  Toast,
  ToastAction,
  ToastClose,
  ToastDescription,
  ToastProvider,
  ToastTitle,
  ToastViewport
} from '.'
import { Button } from '../button'
import { useToast } from '@/hooks/use-toast'

const meta: Meta<typeof Toast> = {
  title: 'Primitives/Toast',
  component: Toast
}

export default meta

function ToastStory({
  title,
  description,
  action,
  ...args
}: StoryObj<typeof Toast> & {
  title?: string
  description?: string
  action?: React.ReactNode
}) {
  return (
    <div className="flex h-36 flex-col items-center justify-center">
      <ToastProvider>
        <Toast {...args}>
          <div className="grid gap-1">
            {title && <ToastTitle>{title}</ToastTitle>}
            {description && <ToastDescription>{description}</ToastDescription>}
          </div>
          {action}
          <ToastClose />
        </Toast>
        <ToastViewport />
      </ToastProvider>
    </div>
  )
}

export const Default: StoryObj<typeof Toast> = {
  render: (args) => {
    const { toast } = useToast()

    const handleClick = () => {
      toast({
        title: 'Toast Title',
        description: 'Toast Description',
        action: <ToastAction altText="Action">Action</ToastAction>,
        variant: args.variant
      })
    }

    return (
      <div className="flex h-48 flex-col items-center justify-center">
        <Button onClick={handleClick}>Show Toast</Button>
        <Toaster />
      </div>
    )
  }
}

export const DefaultToast: StoryObj<typeof Toast> = {
  args: {
    duration: Infinity
  },
  render: (args) => (
    <ToastStory {...args} title="Toast Title" description="Toast Description" />
  )
}

export const DefaultToastWithAction: StoryObj<typeof Toast> = {
  args: {
    duration: Infinity
  },
  render: (args) => (
    <ToastStory
      {...args}
      title="Toast Title"
      description="Toast Description"
      action={<ToastAction altText="Action">Action</ToastAction>}
    />
  )
}

export const SuccessToast: StoryObj<typeof Toast> = {
  args: {
    duration: Infinity,
    variant: 'success'
  },
  render: (args) => (
    <ToastStory {...args} title="Toast Title" description="Toast Description" />
  )
}

export const DestructiveToast: StoryObj<typeof Toast> = {
  args: {
    duration: Infinity,
    variant: 'destructive'
  },
  render: (args) => (
    <ToastStory {...args} title="Toast Title" description="Toast Description" />
  )
}
