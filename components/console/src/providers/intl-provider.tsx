'use client'

import { ReactNode } from 'react'
import { IntlProvider as ReactIntlProvider } from 'react-intl'

// Import language files (you'll need to create these)
const messages = {
  en: {
    'dashboard.title': 'Dashboard',
    'dashboard.welcome': 'Welcome back',
    'transactions.title': 'Transactions',
    'accounts.title': 'Accounts',
    'common.search': 'Search...',
    'common.create': 'Create',
    'common.edit': 'Edit',
    'common.delete': 'Delete',
    'common.save': 'Save',
    'common.cancel': 'Cancel',
  },
  // Add more languages as needed
}

export function IntlProvider({ children }: { children: ReactNode }) {
  // You can get locale from user preferences or browser
  const locale = 'en'
  
  return (
    <ReactIntlProvider 
      locale={locale} 
      messages={messages[locale]} 
      defaultLocale="en"
    >
      {children}
    </ReactIntlProvider>
  )
}