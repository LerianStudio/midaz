import { pathToRegexp } from 'path-to-regexp'

export function checkWhitelist(pathname: string, paths: string[]) {
  return paths.some((path) => pathToRegexp(path).regexp.test(pathname))
}
