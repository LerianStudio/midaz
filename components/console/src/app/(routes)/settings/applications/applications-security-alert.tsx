'use client'

import React from 'react'
import { useIntl, FormattedMessage } from 'react-intl'
import { AlertTriangle } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

export const ApplicationsSecurityAlert = () => {
  const intl = useIntl()

  return (
    <Alert variant="warning" className="mb-6 border-yellow-500/50">
      <AlertTriangle size={24} />
      <AlertTitle className="ml-2 text-sm font-bold text-yellow-800">
        Security Warning
      </AlertTitle>
      <AlertDescription className="text-sm text-yellow-800 opacity-70">
        <ul className="ml-5 mt-2 list-disc space-y-1">
          <li className="font-bold">
            {intl.formatMessage({
              id: 'applications.security.doNotShare',
              defaultMessage:
                'Do not share your clientId or clientSecret publicly. These credentials grant access to your application and must be kept confidential.'
            })}
          </li>
          <li>
            {intl.formatMessage({
              id: 'applications.security.secureStorage',
              defaultMessage: 'Store these keys in a secure location.'
            })}
          </li>
          <li>
            <FormattedMessage
              id="applications.security.doNotDelete"
              defaultMessage="{doNotDelete} the application unless you're sure. Deleting it revokes access to all connected services."
              values={{
                doNotDelete: (chunks) => (
                  <span className="font-bold">{chunks}</span>
                )
              }}
            />
          </li>
          <li>
            {intl.formatMessage({
              id: 'applications.security.rotateCredentials',
              defaultMessage:
                'Rotate your credentials if you suspect they were compromised.'
            })}
          </li>
          <li>
            <FormattedMessage
              id="applications.security.neverExpose"
              defaultMessage="{neverExpose} these keys in frontend code or public repositories."
              values={{
                neverExpose: (chunks) => (
                  <span className="font-bold">{chunks}</span>
                )
              }}
            />
          </li>
        </ul>
      </AlertDescription>
    </Alert>
  )
}
