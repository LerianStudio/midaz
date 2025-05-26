/**
 * Script that runs before DOM mounting to apply changes on client side
 * @param accentColor
 */
export const script = (
  accentColorKey: string,
  accentForegroundColorKey: string
) => {
  const accentColor = localStorage.getItem(accentColorKey)
  const accentForegroundColor = localStorage.getItem(accentForegroundColorKey)

  if (accentColor === '') {
    return
  }

  document.documentElement.style.setProperty('--accent', accentColor)
  document.documentElement.style.setProperty(
    '--accent-foreground',
    accentForegroundColor
  )
}
