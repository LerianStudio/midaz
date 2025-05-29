import React from 'react'
import { Meta, StoryObj } from '@storybook/nextjs'
import { PaginationLimitField, PaginationLimitFieldProps } from '.'
import { useForm } from 'react-hook-form'
import { Form } from '@/components/ui/form'

const meta: Meta<PaginationLimitFieldProps> = {
  title: 'Components/Form/PaginationLimitField',
  component: PaginationLimitField,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<PaginationLimitFieldProps> = {
  render: (args) => {
    const form = useForm({
      defaultValues: { limit: '10' }
    })

    return (
      <Form {...form}>
        <PaginationLimitField {...args} control={form.control} />
      </Form>
    )
  }
}
