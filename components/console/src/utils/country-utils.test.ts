import * as countryUtils from './country-utils'

jest.mock('../../public/countries.json', () => [
  {
    code2: 'US',
    name: 'United States',
    states: [
      { name: 'California', code: 'CA' },
      { name: 'New York', code: 'NY' }
    ]
  },
  {
    code2: 'CA',
    name: 'Canada',
    states: [
      { name: 'Ontario', code: 'ON' },
      { name: 'Quebec', code: 'QC' }
    ]
  }
])

describe('Country Utils', () => {
  describe('getCountries', () => {
    it('should return array of countries with correct structure', () => {
      const countries = countryUtils.getCountries()

      expect(Array.isArray(countries)).toBe(true)
      expect(countries.length).toBe(2)

      expect(countries[0]).toHaveProperty('code', 'US')
      expect(countries[0]).toHaveProperty('name', 'United States')
      expect(countries[0]).toHaveProperty('states')
      expect(Array.isArray(countries[0].states)).toBe(true)
    })

    it('should map country states correctly', () => {
      const countries = countryUtils.getCountries()
      const usStates = countries[0].states

      expect(usStates.length).toBe(2)
      expect(usStates[0]).toEqual({ name: 'California', code: 'CA' })
      expect(usStates[1]).toEqual({ name: 'New York', code: 'NY' })
    })
  })

  describe('getStateCountry', () => {
    it('should return states when providing a valid country code', () => {
      const states = countryUtils.getStateCountry('US')

      expect(Array.isArray(states)).toBe(true)
      expect(states.length).toBe(2)
      expect(states[0]).toEqual({ name: 'California', code: 'CA' })
      expect(states[1]).toEqual({ name: 'New York', code: 'NY' })
    })

    it('should return states when providing a valid country name', () => {
      const states = countryUtils.getStateCountry('Canada')

      expect(Array.isArray(states)).toBe(true)
      expect(states.length).toBe(2)
      expect(states[0]).toEqual({ name: 'Ontario', code: 'ON' })
      expect(states[1]).toEqual({ name: 'Quebec', code: 'QC' })
    })

    it('should return an empty array when providing an invalid country', () => {
      const states = countryUtils.getStateCountry('InvalidCountry')

      expect(Array.isArray(states)).toBe(true)
      expect(states.length).toBe(0)
    })
  })
})
