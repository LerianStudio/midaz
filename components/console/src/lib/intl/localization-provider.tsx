import React from 'react'
import { getIntl } from './get-intl'
import { ClientLocalizationProvider } from './client-localization-provider'

export const LocalizationProvider = async ({
  children
}: React.PropsWithChildren) => {
  const intl = await getIntl()

  return (
    <ClientLocalizationProvider locale={intl.locale} messages={intl.messages}>
      {children}
    </ClientLocalizationProvider>
  )
}
