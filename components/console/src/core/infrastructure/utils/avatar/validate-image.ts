import { getIntl } from '@/lib/intl'
import { downloadFile } from '../files/download-file'
import { validateSVG } from '../svgs/validate-svg'
import { BadRequestApiException } from '@/lib/http'
import { IntlShape } from 'react-intl'

/**
 * Main function to validate an avatar. Performs multiple validation steps:
 * 1. Validates if the avatar is a valid base64 image
 * 2. Validates if the image format is allowed
 * 3. If SVG, validates for secure content
 *
 * @param avatar - Base64 encoded image string with mime type (data:image/format;base64,...)
 * @throws {BadRequestApiException} When validation fails
 */
export async function validateImage(
  avatar: string,
  intl: IntlShape
): Promise<void> {
  try {
    // check if is a base64 image with mime type and extension permitted
    const format = validateImageBase64(avatar, intl)

    // check if is a image format permitted
    validateImageFormat(format, intl)

    // check if is a svg and validate its content
    if (format === 'svg') {
      await validateSvgContent(avatar, intl)
    }
  } catch (error) {
    console.error('[validateAvatar] error', error)
    throw error
  }
}

/**
 * Validates if the provided string is a valid base64 encoded image with proper mime type.
 *
 * @param avatar - Base64 encoded image string to validate
 * @param intl - Internationalization object for error messages
 * @throws {BadRequestApiException} When the avatar is not a valid base64 image
 */
function validateImageBase64(avatar: string, intl: IntlShape): string {
  const base64Regex = /^data:image\/([a-zA-Z0-9.+-]+);base64,(.+)$/

  const match = avatar.match(base64Regex)

  if (!match || !match[1]) {
    throw new BadRequestApiException(
      intl.formatMessage({
        id: 'error.api.avatarInvalidFormat',
        defaultMessage: 'Avatar is not a valid mime type.'
      })
    )
  }

  return match[1]
}

/**
 * Validates if the image format is allowed based on environment configuration.
 * Extracts the format from the base64 mime type and checks against NEXT_PUBLIC_MIDAZ_CONSOLE_AVATAR_ALLOWED_FORMAT.
 *
 * @param format - Format of the image to validate
 * @param intl - Internationalization object for error messages
 * @returns The validated image format
 * @throws {BadRequestApiException} When the image format is not allowed
 */
function validateImageFormat(format: string, intl: IntlShape): string {
  const allowedFormats =
    process.env.NEXT_PUBLIC_MIDAZ_CONSOLE_AVATAR_ALLOWED_FORMAT?.split(',').map(
      (e) => e.trim().toLowerCase()
    ) ?? process.env.NEXT_PUBLIC_MIDAZ_CONSOLE_AVATAR_ALLOWED_FORMAT?.split(',')

  if (!allowedFormats?.includes(format)) {
    throw new BadRequestApiException(
      intl.formatMessage({
        id: 'error.api.avatarExtensionNotAllowed',
        defaultMessage: 'Avatar is not a permitted extension file.'
      })
    )
  }

  return format
}

/**
 * Validates SVG content for security. Downloads the SVG file and checks for unsafe content.
 *
 * @param avatar - Base64 encoded SVG string to validate
 * @param intl - Internationalization object for error messages
 * @throws {BadRequestApiException} When the SVG contains unsafe content
 */
async function validateSvgContent(
  avatar: string,
  intl: IntlShape
): Promise<void> {
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
