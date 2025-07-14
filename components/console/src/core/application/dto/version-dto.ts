export enum VersionStatus {
  UpToDate = 'up-to-date',
  Outdated = 'outdated'
}

export type VersionDto = {
  current: string
  latest: string
  status: VersionStatus
}
