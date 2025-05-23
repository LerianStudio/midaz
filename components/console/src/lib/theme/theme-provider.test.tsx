import { act, render, screen } from '@testing-library/react'
import { ThemeProvider, ThemeState, useTheme } from './theme-provider'

function TestComponent({ themeParam }: { themeParam?: Partial<ThemeState> }) {
  const { accentColor, logoUrl, setTheme } = useTheme()

  return (
    <>
      <p data-testid="accentColor">{accentColor}</p>
      <p data-testid="logoUrl">{logoUrl}</p>
      <button data-testid="change" onClick={() => setTheme(themeParam!)} />
    </>
  )
}

function setup(themeParam?: Partial<ThemeState>) {
  render(
    <ThemeProvider>
      <TestComponent themeParam={themeParam} />
    </ThemeProvider>
  )
  return {
    accentColor: screen.getByTestId('accentColor'),
    logoUrl: screen.getByTestId('logoUrl'),
    change: screen.getByTestId('change')
  }
}

describe('ThemeProvider', () => {
  const color = '#FFFFFF'
  const url = '/test.png'

  const mockGet = jest.spyOn(Storage.prototype, 'getItem')
  const mockSet = jest.spyOn(Storage.prototype, 'setItem')

  beforeEach(() => {
    mockGet.mockImplementation(() => '')
    mockSet.mockImplementation(() => {})
  })

  test('should render properly', () => {
    const { accentColor, logoUrl, change } = setup()

    expect(accentColor).toBeDefined()
    expect(logoUrl).toBeDefined()
    expect(change).toBeDefined()
  })

  test('should change accent color', async () => {
    const { accentColor, change } = setup({ accentColor: color })

    expect(accentColor.innerHTML).toBe('')

    await act(() => change.click())

    expect(accentColor.innerHTML).toBe(color)
  })

  test('should change logo URL', async () => {
    const { logoUrl, change } = setup({ logoUrl: url })

    expect(logoUrl.innerHTML).toBe('')

    await act(() => change.click())

    expect(logoUrl.innerHTML).toBe(url)
  })

  test('should load from LocalStorage', async () => {
    mockGet.mockImplementation((key) => {
      if (key === 'accentColor') {
        return color
      }
      if (key === 'logoUrl') {
        return url
      }

      return ''
    })

    const { accentColor, logoUrl } = setup()

    expect(accentColor.innerHTML).toBe(color)
    expect(logoUrl.innerHTML).toBe(url)
  })

  test('should save into LocalStorage', async () => {
    const { accentColor, change } = setup({ accentColor: color })

    expect(accentColor.innerHTML).toBe('')

    await act(() => change.click())

    expect(accentColor.innerHTML).toBe(color)

    expect(mockSet).toHaveBeenCalledWith('accentColor', color)
  })
})
