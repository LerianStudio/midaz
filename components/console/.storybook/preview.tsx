import type { Preview } from '@storybook/react'

import '../src/app/globals.css'
import './storybook.css'

import { ThemeProvider } from '../src/lib/theme/theme-provider'
import { IntlProvider } from 'react-intl'

const preview: Preview = {
  parameters: {
    backgrounds: {
      values: [{ name: 'Light', value: '#f4f4f5' }],
      default: 'Light'
    },
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i
      }
    }
  }
}

export const decorators = [
  (Story) => (
    <ThemeProvider>
      <IntlProvider locale="en">
        <Story />
      </IntlProvider>
    </ThemeProvider>
  )
]

export default preview
