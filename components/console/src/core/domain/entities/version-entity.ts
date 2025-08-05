export enum VersionStatus {
  UpToDate = 'up-to-date',
  Outdated = 'outdated'
}

export type VersionEntity = {
  latest: string
}
