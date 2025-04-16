import get from 'lodash/get'
import { IntlShape } from 'react-intl'
import {
  defaultErrorMap,
  ErrorMapCtx,
  ZodIssueCode,
  ZodIssueOptionalMessage,
  ZodParsedType
} from 'zod'
import messages from './messages'
import dayjs from 'dayjs'

/**
 * TODO: Proper implement this
 * @param intl
 * @returns
 */
export default function createZodMap(intl: IntlShape) {
  return (issue: ZodIssueOptionalMessage, ctx: ErrorMapCtx) => {
    let message: string = defaultErrorMap(issue, ctx).message

    switch (issue.code) {
      case ZodIssueCode.invalid_type:
        if (issue.received === ZodParsedType.undefined) {
          message = intl.formatMessage(messages.invalid_type_received_undefined)
        }
        break
      case ZodIssueCode.too_small:
        const minimum =
          issue.type === 'date'
            ? dayjs.unix(issue.minimum as number).format('LL')
            : issue.minimum
        const precisionMinimum = issue.exact
          ? 'exact'
          : issue.inclusive
            ? 'inclusive'
            : 'not_inclusive'
        const keyMinimum = `too_small_${issue.type}_${precisionMinimum}`

        if (!(keyMinimum in messages)) {
          throw new Error(
            `Zod Intl: ${keyMinimum} id is not defined in messages`
          )
        }

        // TODO: review this error
        // @ts-ignore
        message = intl.formatMessage(messages[keyMinimum], { minimum })
        break
      case ZodIssueCode.too_big:
        const maximum =
          issue.type === 'date'
            ? dayjs.unix(issue.maximum as number).format('LL')
            : issue.maximum
        const precisionMaximum = issue.exact
          ? 'exact'
          : issue.inclusive
            ? 'inclusive'
            : 'not_inclusive'
        const keyMaximum = `too_big_${issue.type}_${precisionMaximum}`

        if (!(keyMaximum in messages)) {
          throw new Error(
            `Zod Intl: ${keyMaximum} id is not defined in messages`
          )
        }

        // TODO: review this error
        // @ts-ignore
        message = intl.formatMessage(messages[keyMaximum], { maximum })
        break
      case ZodIssueCode.custom:
        if (!issue?.params?.id) {
          throw new Error(
            `Zod Intl: Custom validation with path ${issue.path} has params.id undefined`
          )
        }

        if (!(issue.params.id in messages)) {
          throw new Error(
            `Zod Intl: ${issue.params.id} id is not defined in messages`
          )
        }

        message = intl.formatMessage(
          get(messages, issue.params.id),
          issue.params
        )
        break
    }
    return { message }
  }
}
