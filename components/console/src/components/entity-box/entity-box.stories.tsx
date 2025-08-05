import React from 'react'
import { Meta, StoryObj } from '@storybook/nextjs'
import { EntityBox } from '.'
import { Button } from '../ui/button'
import { useForm } from 'react-hook-form'
import { InputField } from '../form'
import { PaginationLimitField } from '../form/pagination-limit-field'
import { Form } from '../ui/form'

const meta: Meta = {
  title: 'Components/EntityBox',
  component: EntityBox.Root,
  parameters: {
    backgrounds: {
      default: 'Light'
    }
  },
  argTypes: {}
}

export default meta

export const Primary: StoryObj = {
  render: (args) => (
    <EntityBox.Root {...args}>
      <EntityBox.Header
        title="Ledgers"
        subtitle="Manage the ledgers of this organization."
        tooltip="Clustering or allocation of customers at different levels."
      />
      <EntityBox.Actions>
        <Button>New Ledger</Button>
      </EntityBox.Actions>
    </EntityBox.Root>
  )
}

export const Collapsible: StoryObj = {
  render: (args) => {
    const form = useForm({ defaultValues: { limit: '10', name: '' } })

    return (
      <Form {...form}>
        <EntityBox.Collapsible {...args}>
          <EntityBox.Banner>
            <EntityBox.Header
              title="Ledgers"
              subtitle="Manage the ledgers of this organization."
              tooltip="Clustering or allocation of customers at different levels."
            />
            <div className="col-start-2 flex flex-row items-center justify-center">
              <InputField
                name="name"
                placeholder="Search..."
                control={form.control}
              />
            </div>
            <EntityBox.Actions>
              <Button>New Ledger</Button>
              <EntityBox.CollapsibleTrigger />
            </EntityBox.Actions>
          </EntityBox.Banner>
          <EntityBox.CollapsibleContent>
            <div className="col-start-3 flex justify-end">
              <PaginationLimitField control={form.control} />
            </div>
          </EntityBox.CollapsibleContent>
        </EntityBox.Collapsible>
      </Form>
    )
  }
}
