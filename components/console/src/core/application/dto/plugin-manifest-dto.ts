export type CreatePluginManifestDto = {
  host: string
}

export type PluginManifestDto = {
  id: string
  name: string
  title: string
  description: string
  version: string
  route: string
  icon: string
  enabled: boolean
  entry: string
  healthcheck: string
  host: string
  author: string
}
