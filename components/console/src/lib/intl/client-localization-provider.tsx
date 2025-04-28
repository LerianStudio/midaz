'use client'

import { IntlProvider } from 'react-intl'

interface ClientLocalizationProviderProps extends React.PropsWithChildren {
  locale: string
  messages: React.ComponentProps<typeof IntlProvider>['messages']
}

/**
 * Client side of LocalizationProvider, to allow hooks usage on client side components
 */
export const ClientLocalizationProvider = ({
  messages,
  locale,
  children
}: ClientLocalizationProviderProps) => {
  return (
    <IntlProvider messages={messages} locale={locale} onError={() => {}}>
      {children}
    </IntlProvider>
  )
}
