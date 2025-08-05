import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger
} from '.'
import { Meta, StoryObj } from '@storybook/nextjs'
import { Button } from '../button'

const meta: Meta = {
  title: 'Primitives/Sheet',
  component: Sheet,
  argTypes: {}
}

export default meta

export const Primary: StoryObj = {
  render: (args) => (
    <Sheet {...args}>
      <SheetTrigger>Open</SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Are you absolutely sure?</SheetTitle>
          <SheetDescription>
            This action cannot be undone. This will permanently delete your
            account and remove your data from our servers.
          </SheetDescription>
        </SheetHeader>
      </SheetContent>
    </Sheet>
  )
}

const SHEET_SIDES = ['top', 'right', 'bottom', 'left'] as const

type SheetSide = (typeof SHEET_SIDES)[number]

export const SheetSide: StoryObj = {
  render: (args) => {
    return (
      <div className="grid grid-cols-2 gap-2">
        {SHEET_SIDES.map((side) => (
          <Sheet key={side} {...args}>
            <SheetTrigger asChild>
              <Button variant="outline">{side}</Button>
            </SheetTrigger>
            <SheetContent side={side}>
              <SheetHeader>
                <SheetTitle>Edit profile</SheetTitle>
                <SheetDescription>
                  Make changes to your profile here. Click save when you are
                  done.
                </SheetDescription>
              </SheetHeader>
              <div className="grid gap-4 py-4">
                <p>Content</p>
                <p>Content</p>
                <p>Content</p>
                <p>Content</p>
              </div>
              <SheetFooter>
                <SheetClose asChild>
                  <Button type="submit">Save changes</Button>
                </SheetClose>
              </SheetFooter>
            </SheetContent>
          </Sheet>
        ))}
      </div>
    )
  }
}
