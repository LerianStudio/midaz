import fs from 'fs'

const getValue = (file: string, variable: string) => {
  const match = file.match(new RegExp(`^${variable}=(.*)$`, 'm'))
  const value = match ? match[1] : null
  return value?.replaceAll("'", '')
}

describe('.env.example', () => {
  let envExample: string

  beforeAll(() => {
    envExample = fs.readFileSync('.env.example', 'utf-8')
  })

  it('should have NEXTAUTH_SECRET set to "SECRET"', () => {
    expect(getValue(envExample, 'NEXTAUTH_SECRET')).toBe('SECRET')
  })

  it('should have PLUGIN_AUTH_CLIENT_ID set to "SECRET"', () => {
    expect(getValue(envExample, 'PLUGIN_AUTH_CLIENT_ID')).toBe('SECRET')
  })

  it('should have PLUGIN_AUTH_CLIENT_SECRET set to "SECRET"', () => {
    expect(getValue(envExample, 'PLUGIN_AUTH_CLIENT_SECRET')).toBe('SECRET')
  })
})
