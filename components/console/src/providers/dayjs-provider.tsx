'use client'

import React from 'react'
import dayjs from 'dayjs'
import localizedFormat from 'dayjs/plugin/localizedFormat'
import updateLocale from 'dayjs/plugin/updateLocale'
import relativeTime from 'dayjs/plugin/relativeTime'

import 'dayjs/locale/pt'
import 'dayjs/locale/es-us'
import { useIntl } from 'react-intl'

const DayjsProvider = ({ children }: React.PropsWithChildren) => {
  const intl = useIntl()

  React.useEffect(() => {
    dayjs.extend(localizedFormat)
    dayjs.extend(updateLocale)
    dayjs.extend(relativeTime)
  }, [])

  React.useEffect(() => {
    dayjs.locale(intl.locale.toLowerCase())
  }, [intl.locale])

  return <React.Fragment>{children}</React.Fragment>
}

export default DayjsProvider
