import { defineMessages } from 'react-intl'

const messages = defineMessages({
  invalid_type: {
    id: 'errors.invalid_type',
    defaultMessage: ''
  },
  invalid_type_received_undefined: {
    id: 'errors.invalid_type_received_undefined',
    defaultMessage: 'Required field'
  },

  too_small_string_exact: {
    id: 'errors.too_small.string.exact',
    defaultMessage:
      'Field must contain exactly {minimum} {minimum, plural, =0 {characters} one {character} other {characters}}'
  },
  too_small_string_inclusive: {
    id: 'errors.too_small.string.inclusive',
    defaultMessage:
      'Field must contain at least {minimum} {minimum, plural, =0 {characters} one {character} other {characters}}'
  },
  too_small_string_not_inclusive: {
    id: 'errors.too_small.string.not_inclusive',
    defaultMessage:
      'Field must contain over {minimum} {minimum, plural, =0 {characters} one {character} other {characters}}'
  },
  too_small_number_not_inclusive: {
    id: 'errors.too_small.number.not_inclusive',
    defaultMessage: 'Field must be greater than {minimum}'
  },
  too_small_date_exact: {
    id: 'errors.too_small.date.exact',
    defaultMessage: 'Date must be exactly {minimum}'
  },
  too_small_date_inclusive: {
    id: 'errors.too_small.date.inclusive',
    defaultMessage: 'Date must be after or equal to {minimum}'
  },
  too_small_date_not_inclusive: {
    id: 'errors.too_small.date.not_inclusive',
    defaultMessage: 'Date must be after {minimum}'
  },
  too_small_array_inclusive: {
    id: 'errors.too_small.array.inclusive',
    defaultMessage:
      'Field must contain at least {minimum} {minimum, plural, =0 {items} one {item} other {items}}'
  },

  too_big_string_exact: {
    id: 'errors.too_big.string.exact',
    defaultMessage:
      'Field must contain exactly {maximum} {maximum, plural, =0 {characters} one {character} other {characters}}'
  },
  too_big_string_inclusive: {
    id: 'errors.too_big.string.inclusive',
    defaultMessage:
      'Field must contain at most {maximum} {maximum, plural, =0 {characters} one {character} other {characters}}'
  },
  too_big_string_not_inclusive: {
    id: 'errors.too_big.string.not_inclusive',
    defaultMessage:
      'Field must contain under {maximum} {maximum, plural, =0 {characters} one {character} other {characters}}'
  },
  too_big_number_inclusive: {
    id: 'errors.too_big.number.inclusive',
    defaultMessage: 'Field must be less than or equal to {maximum}'
  },
  too_big_date_exact: {
    id: 'errors.too_big.date.exact',
    defaultMessage: 'Date must be exactly {maximum}'
  },
  too_big_date_inclusive: {
    id: 'errors.too_big.date.inclusive',
    defaultMessage: 'Date must be before or equal to {maximum}'
  },
  too_big_date_not_inclusive: {
    id: 'errors.too_big.date.not_inclusive',
    defaultMessage: 'Date must be before {maximum}'
  },

  custom_special_characters: {
    id: 'errors.custom.special_characters',
    defaultMessage: 'Field must not contain special characters'
  },
  custom_one_uppercase_letter: {
    id: 'errors.custom.one_uppercase_letter',
    defaultMessage: 'Field must contain at least 1 uppercase letter'
  },
  custom_one_lowercase_letter: {
    id: 'errors.custom.one_lowercase_letter',
    defaultMessage: 'Field must contain at least 1 lowercase letter'
  },
  custom_one_number: {
    id: 'errors.custom.one_number',
    defaultMessage: 'Field must contain at least 1 number'
  },
  custom_one_special_character: {
    id: 'errors.custom.one_special_character',
    defaultMessage: 'Field must contain at least 1 special character'
  },
  custom_only_numbers: {
    id: 'errors.custom.only_numbers',
    defaultMessage: 'Field must contain only numbers'
  },
  custom_date_invalid: {
    id: 'errors.custom.date.invalid',
    defaultMessage: 'Invalid date'
  },
  custom_uppercase_required: {
    id: 'errors.custom.uppercase_required',
    defaultMessage: 'Field must be in uppercase and consist of letters only'
  },
  custom_alphanumeric_with_dash_underscore: {
    id: 'errors.custom.alphanumeric_with_dash_underscore',
    defaultMessage:
      'Field must contain only letters, numbers, hyphens, and underscores'
  },
  custom_avatar_invalid_format: {
    id: 'errors.custom.avatar.invalid_format',
    defaultMessage: 'Avatar should have a {format} format',
    values: {
      format: ['png', 'svg']
    }
  },
  custom_confirm_password: {
    id: 'errors.custom.confirm_password',
    defaultMessage: 'Passwords do not match'
  }
})

export default messages
