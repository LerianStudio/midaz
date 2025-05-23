'use client'

/**
 * Gets the current search params from the URL as an object
 *
 * Beware: window is not defined in the server on the first render, even on client components.
 * Use useSearchParams hook instead.
 * @returns Object with search params
 */
export function getSearchParams() {
  return Object.fromEntries(new URLSearchParams(window.location.search))
}
