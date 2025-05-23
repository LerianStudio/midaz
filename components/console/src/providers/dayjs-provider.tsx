'use client'

import React from 'react'
import dayjs from 'dayjs'
import localizedFormat from 'dayjs/plugin/localizedFormat'
import updateLocale from 'dayjs/plugin/updateLocale'

import 'dayjs/locale/pt'
import 'dayjs/locale/es-us'
import { useIntl } from 'react-intl'

const DayjsProvider = ({ children }: React.PropsWithChildren) => {
  const intl = useIntl()

  React.useEffect(() => {
    dayjs.extend(localizedFormat)
    dayjs.extend(updateLocale)
  }, [])

  React.useEffect(() => {
    dayjs.locale(intl.locale.toLowerCase())
  }, [intl.locale])

  return <React.Fragment>{children}</React.Fragment>
}

export default DayjsProvider
