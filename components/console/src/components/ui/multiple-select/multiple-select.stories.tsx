import { Meta, StoryObj } from '@storybook/react'
import React from 'react'
import {
  MultipleSelect,
  MultipleSelectContent,
  MultipleSelectEmpty,
  MultipleSelectItem,
  MultipleSelectTrigger,
  MultipleSelectValue
} from '.'

const meta: Meta = {
  title: 'Primitives/MultipleSelect',
  component: MultipleSelect,
  argTypes: {}
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
  },
  {
    value: 'vue.js',
    label: 'Vue.js'
  },
  {
    value: 'angular',
    label: 'Angular'
  },
  {
    value: 'ember.js',
    label: 'Ember.js'
  },
  {
    value: 'backbone.js',
    label: 'Backbone.js'
  },
  {
    value: 'jquery',
    label: 'jQuery'
  },
  {
    value: 'vanilla-js',
    label: 'Vanilla JS'
  },
  {
    value: 'lit',
    label: 'Lit'
  },
  {
    value: 'preact',
    label: 'Preact'
  },
  {
    value: 'solidjs',
    label: 'SolidJS'
  },
  {
    value: 'svelte',
    label: 'Svelte'
  },
  {
    value: 'jquery-mobile',
    label: 'jQuery Mobile'
  },
  {
    value: 'jquery-ui',
    label: 'jQuery UI'
  },
  {
    value: 'jquery-mobile-v1',
    label: 'jQuery Mobile v1'
  }
]

const BaseLayout = ({ children }: React.PropsWithChildren) => (
  <div className="h-[1024px]">{children}</div>
)

const Component = (args: any) => {
  return (
    <BaseLayout>
      <MultipleSelect {...args}>
        <MultipleSelectTrigger>
          <MultipleSelectValue placeholder="Select a framework..." />
        </MultipleSelectTrigger>
        <MultipleSelectContent>
          <MultipleSelectEmpty>
            <p className="text-sm text-muted-foreground">No frameworks found</p>
          </MultipleSelectEmpty>
          {frameworks.map((framework) => (
            <MultipleSelectItem key={framework.value} value={framework.value}>
              {framework.label}
            </MultipleSelectItem>
          ))}
        </MultipleSelectContent>
      </MultipleSelect>
    </BaseLayout>
  )
}

export const Primary: StoryObj = {
  render: (args) => <Component {...args} />
}

export const WithValues: StoryObj = {
  args: {
    showValue: true
  },
  render: (args) => <Component {...args} />
}

export const Disabled: StoryObj = {
  args: {
    disabled: true
  },
  render: (args) => <Component {...args} />
}
