'use client'

import React from 'react'
import { script } from './script'
import { isNil } from 'lodash'
import Color from 'colorjs.io'

const isServer = typeof window === 'undefined'

export type ThemeState = {
  logoUrl: string
  accentColor: string
}

type ThemeContextProps = ThemeState & {
  setTheme: (theme: Partial<ThemeState>) => void
}

const logoUrlKey = 'logoUrl'
const accentColorKey = 'accentColor'
const accentForegroundColorKey = 'accentForegroundColor'

const defaultContext: ThemeContextProps = {
  logoUrl: '',
  accentColor: '',
  setTheme: (_) => {}
}

const ThemeContext = React.createContext<ThemeContextProps>(defaultContext)

export const useTheme = () => React.useContext(ThemeContext) ?? defaultContext

export const ThemeProvider = ({ children }: React.PropsWithChildren) => {
  const [theme, _setTheme] = React.useReducer(
    (prev: ThemeState, state: Partial<ThemeState>) => ({ ...prev, ...state }),
    {
      logoUrl: getStorage(logoUrlKey, defaultContext.logoUrl),
      accentColor: getStorage(accentColorKey, defaultContext.accentColor)
    }
  )

  React.useEffect(() => {
    const handleStorage = (e: StorageEvent) => {
      if (e.key === accentColorKey) {
        _setTheme({
          [accentColorKey]: e.newValue || defaultContext.accentColor
        })
      }
      if (e.key === logoUrlKey) {
        _setTheme({ [logoUrlKey]: e.newValue || defaultContext.logoUrl })
      }
    }

    window.addEventListener('storage', handleStorage)

    return () => {
      window.removeEventListener('storage', handleStorage)
    }
  }, [_setTheme])

  const _save = (theme: Partial<ThemeState>) => {
    try {
      if (!isNil(theme.accentColor)) {
        const accentForegroundColor = getContrastColor(theme.accentColor)

        localStorage.setItem(accentColorKey, theme.accentColor)
        localStorage.setItem(accentForegroundColorKey, accentForegroundColor)

        document.documentElement.style.setProperty(
          '--accent',
          theme.accentColor
        )
        document.documentElement.style.setProperty(
          '--accent-foreground',
          accentForegroundColor
        )
      }

      if (!isNil(theme.logoUrl)) {
        localStorage.setItem(logoUrlKey, theme.logoUrl)
      }
    } catch {}
  }

  React.useEffect(() => {
    _save(theme)
  }, [theme])

  return (
    <>
      <script
        suppressHydrationWarning
        dangerouslySetInnerHTML={{
          __html: `(${script.toString()})("${accentColorKey}", "${accentForegroundColorKey}")`
        }}
      />
      <ThemeContext.Provider value={{ ...theme, setTheme: _setTheme }}>
        {children}
      </ThemeContext.Provider>
    </>
  )
}

const getStorage = (key: string, defaultValue: string) => {
  if (isServer) {
    return defaultValue
  }

  let value
  try {
    value = localStorage.getItem(key) || undefined
  } catch {}
  return value || defaultValue
}

const getContrastColor = (color: string) => {
  if (isNil(color) || color === '') {
    return ''
  }

  const black = '0 0 0%'
  const white = '0 0 100%'

  // This case might happen if the user mistakenly modify localStorage with wrong values
  try {
    const colorObject = new Color(`hsl(${color})`)

    // APAC algorith returns a maximum of 110 when comparing black or white,
    // But this values is polarized, so the true range is between -110 and 110,
    // returns black as foreground color
    return Math.abs(Color.contrastAPCA(colorObject, new Color('black'))) >
      Math.abs(Color.contrastAPCA(colorObject, new Color('white')))
      ? black
      : white
  } catch {
    return ''
  }
}
