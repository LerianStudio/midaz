import { z } from 'zod'
import messages from '@/lib/zod/messages'
import {
  regex,
  oneUpperCaseLetter,
  oneLowerCaseLetter,
  oneNumber,
  oneSpecialCharacter
} from './regex'

const firstName = z.string().min(3).max(255)

const lastName = z.string().min(3).max(255)

const username = z.string().min(3).max(255)

const email = z.string().email().max(255)

const password = z
  .string()
  .min(8)
  .max(255)
  .refine(regex(oneUpperCaseLetter), {
    params: { id: 'custom_one_uppercase_letter' }
  })
  .refine(regex(oneLowerCaseLetter), {
    params: { id: 'custom_one_lowercase_letter' }
  })
  .refine(regex(oneNumber), {
    params: { id: 'custom_one_number' }
  })
  .refine(regex(oneSpecialCharacter), {
    params: { id: 'custom_one_special_character' }
  })

const confirmPassword = z.string().min(8).max(255)

const groups = z.array(z.string()).nonempty()

export const user = {
  firstName,
  lastName,
  username,
  email,
  password,
  groups
}

export const passwordChange = {
  confirmPassword
}
