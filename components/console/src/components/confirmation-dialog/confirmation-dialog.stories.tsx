import { Meta } from '@storybook/nextjs'
import ConfirmationDialog, { ConfirmationDialogProps } from '.'
import { Button } from '../ui/button'
import React from 'react'

const meta: Meta<ConfirmationDialogProps> = {
  title: 'Components/ConfirmationDialog',
  component: ConfirmationDialog,
  argTypes: {}
}

export default meta

const Template = (props: ConfirmationDialogProps) => {
  const [open, setOpen] = React.useState(false)

  return (
    <>
      <Button onClick={() => setOpen(true)}>Open</Button>
      <ConfirmationDialog
        title="Are you sure?"
        description="Lorem ipsum dolor sit amet, consectetur adipiscing elit. Phasellus varius convallis nunc, vitae hendrerit tortor commodo sed. Pellentesque quis sapien sollicitudin, venenatis purus vitae, finibus turpis."
        {...props}
        open={open}
        onOpenChange={setOpen}
        onCancel={() => setOpen(false)}
        onConfirm={() => setOpen(false)}
      />
    </>
  )
}

export const Primary = Template.bind({})
