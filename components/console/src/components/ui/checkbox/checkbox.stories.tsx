import { Meta, StoryObj } from '@storybook/nextjs'
import { Checkbox } from '.'
import { CheckboxProps } from '@radix-ui/react-checkbox'

const meta: Meta<CheckboxProps> = {
  title: 'Primitives/Checkbox',
  component: Checkbox,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<CheckboxProps> = {
  render: (args) => <Checkbox {...args} />
}

export const WithText: StoryObj<CheckboxProps> = {
  render: (args) => (
    <div className="flex items-center space-x-2">
      <Checkbox id="terms" {...args} />
      <label
        htmlFor="terms"
        className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
      >
        Accept terms and conditions
      </label>
    </div>
  )
}
