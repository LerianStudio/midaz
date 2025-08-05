import { Meta, StoryObj } from '@storybook/nextjs'
import { Switch } from '.'
import { SwitchProps } from '@radix-ui/react-switch'
import { Label } from '../label'

const meta: Meta<SwitchProps> = {
  title: 'Primitives/Switch',
  component: Switch,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<SwitchProps> = {
  render: (args) => <Switch {...args} />
}

export const WithText: StoryObj<SwitchProps> = {
  render: (args) => (
    <div className="flex items-center space-x-2">
      <Switch id="airplane-mode" {...args} />
      <Label htmlFor="airplane-mode">Airplane Mode</Label>
    </div>
  )
}
