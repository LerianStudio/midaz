import { CommandItem } from '@/components/ui/command'
import { ComboBoxField, ComboBoxFieldProps } from '../combo-box-field'
import { getCountries } from '@/utils/country-utils'

export type CountryFieldProps = Omit<ComboBoxFieldProps, 'children'> & {}

export const CountryField = ({ ...others }: CountryFieldProps) => {
  return (
    <ComboBoxField {...others}>
      {getCountries().map((country) => (
        <CommandItem key={country.code} value={country.code}>
          {country.name}
        </CommandItem>
      ))}
    </ComboBoxField>
  )
}
