export enum VersionStatus {
  UpToDate = 'up-to-date',
  Outdated = 'outdated'
}

export type MidazInfoDto = {
  currentVersion: string
  latestVersion: string
  versionStatus: VersionStatus
}
