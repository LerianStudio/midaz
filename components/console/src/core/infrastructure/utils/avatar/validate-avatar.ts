import { getIntl } from '@/lib/intl'
import { downloadFile } from '../files/download-file'
import { validateSVG } from '../svgs/validate-svg'
import { BadRequestApiException } from '@/lib/http'

/**
 * Validates the avatar URL, checking if it is a SVG and if it contains unsecure content.
 *
 * @param avatar Avatar URL
 */
export async function validateAvatar(avatar: string): Promise<void> {
  const intl = await getIntl()

  if (avatar && avatar.includes('.svg')) {
    const file = await downloadFile(avatar)

    if (!validateSVG(file)) {
      throw new BadRequestApiException(
        intl.formatMessage({
          id: 'error.api.avatarSvgContainsUnsecureContent',
          defaultMessage: 'Avatar SVG contains unsecure content.'
        })
      )
    }
  }
}
