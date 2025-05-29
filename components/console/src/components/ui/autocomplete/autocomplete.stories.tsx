import { Meta, StoryObj } from '@storybook/nextjs'
import React from 'react'
import {
  Autocomplete,
  AutocompleteContent,
  AutocompleteEmpty,
  AutocompleteGroup,
  AutocompleteItem,
  AutocompleteTrigger,
  AutocompleteValue
} from '.'

const meta: Meta = {
  title: 'Primitives/Autocomplete',
  component: Autocomplete
}

export default meta

const frameworks = [
  {
    value: 'next.js',
    label: 'Next.js'
  },
  {
    value: 'sveltekit',
    label: 'SvelteKit'
  },
  {
    value: 'nuxt.js',
    label: 'Nuxt.js'
  },
  {
    value: 'remix',
    label: 'Remix'
  },
  {
    value: 'astro',
    label: 'Astro'
  }
]

const BaseLayout = ({ children }: React.PropsWithChildren) => (
  <div className="h-[256px]">{children}</div>
)

const Component = (args: any) => {
  return (
    <BaseLayout>
      <Autocomplete {...args}>
        <AutocompleteTrigger>
          <AutocompleteValue placeholder="Select a framework" />
        </AutocompleteTrigger>
        <AutocompleteContent>
          <AutocompleteEmpty>
            <p className="text-sm text-muted-foreground">No frameworks found</p>
          </AutocompleteEmpty>
          <AutocompleteGroup>
            {frameworks.map((framework) => (
              <AutocompleteItem key={framework.value} value={framework.value}>
                {framework.label}
              </AutocompleteItem>
            ))}
          </AutocompleteGroup>
        </AutocompleteContent>
      </Autocomplete>
    </BaseLayout>
  )
}

export const Primary: StoryObj = {
  render: (args) => <Component {...args} />
}

export const ShowValues: StoryObj = {
  args: {
    showValue: true
  },
  render: (args) => <Component {...args} />
}

export const ReadOnly: StoryObj = {
  args: {
    value: 'next.js',
    readOnly: true
  },
  render: (args) => <Component {...args} />
}

export const Disabled: StoryObj = {
  args: {
    value: 'next.js',
    disabled: true
  },
  render: (args) => <Component {...args} />
}
