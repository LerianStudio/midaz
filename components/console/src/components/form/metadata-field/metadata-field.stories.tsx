import { Meta, StoryObj } from '@storybook/react'
import { useForm } from 'react-hook-form'
import { MetadataField, MetadataFieldProps } from '.'
import { Form } from '@/components/ui/form'

const meta: Meta<MetadataFieldProps> = {
  title: 'Components/Form/MetadataField',
  component: MetadataField,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<MetadataFieldProps> = {
  parameters: {
    backgrounds: {
      default: 'light'
    }
  },
  render: () => {
    const form = useForm({
      defaultValues: {
        metadata: {}
      }
    })

    return (
      <Form {...form}>
        <form className="w-3/4">
          <MetadataField name="metadata" control={form.control} />
        </form>
      </Form>
    )
  }
}
