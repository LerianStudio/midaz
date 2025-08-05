export type PluginManifestEntity = {
  id?: string
  name: string
  title: string
  description: string
  version: string
  route: string
  entry: string
  healthcheck: string
  host: string
  icon: string
  enabled: boolean
  author: string
}
