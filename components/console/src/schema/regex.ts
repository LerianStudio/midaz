export const regex = (pattern: RegExp) => (value: string) =>
  value.match(pattern)

export const specialCharacters = /^[ a-zA-Z0-9äöüÄÖÜ]*$/

export const oneUpperCaseLetter = /[A-Z]/

export const oneLowerCaseLetter = /[a-z]/

export const oneNumber = /[0-9]/

export const onlyNumbers = /^[0-9]*$/

export const uppercaseLettersOnly = /^[A-Z]+$/

export const oneSpecialCharacter = /[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]/
