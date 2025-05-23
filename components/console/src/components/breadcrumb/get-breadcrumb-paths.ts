import { BreadcrumbPath } from '.'
import { isNil } from 'lodash'

type BreadcrumbPathTabs = (BreadcrumbPath & {
  active?: () => boolean
})[]

export function getBreadcrumbPaths(paths: BreadcrumbPathTabs) {
  return paths
    .filter((path) => {
      if (isNil(path.active)) {
        return true
      }

      return path.active()
    })
    .map(({ active, ...path }) => path)
}
