'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import createZodMap from './create-zod-map'

export default function ZodSchemaProvider({
  children
}: React.PropsWithChildren) {
  const intl = useIntl()

  React.useEffect(() => {
    z.setErrorMap(createZodMap(intl))
  }, [intl])

  return children
}
