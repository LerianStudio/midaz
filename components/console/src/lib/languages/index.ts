import { defineMessage } from 'react-intl'

const languages = [
  {
    name: 'English',
    locale: 'en-US',
    localizedName: defineMessage({
      id: 'language.english',
      defaultMessage: 'English'
    })
  },
  {
    name: 'Português (Brasil)',
    locale: 'pt-BR',
    localizedName: defineMessage({
      id: 'language.portuguese',
      defaultMessage: 'Portuguese'
    })
  }
]

export type Language = typeof languages extends readonly (infer ElementType)[]
  ? ElementType
  : never

export default languages
