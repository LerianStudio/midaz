import { getIntl } from '@/lib/intl'
import { MidazApiException } from '../midaz/exceptions/midaz-exceptions'

export interface MidazErrorData {
  code: string
  message: string
}

export async function handleMidazError(
  midazError: MidazErrorData
): Promise<void> {
  const intl = await getIntl()

  throw new MidazApiException(
    intl.formatMessage({
      id: 'error.midaz.unknowError',
      defaultMessage: 'Unknown error on Midaz.'
    })
  )
}
