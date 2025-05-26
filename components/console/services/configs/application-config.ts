import path from 'path'
import readYamlFile from 'read-yaml-file'
import type {
  Icon,
  Icons,
  IconURL
} from 'next/dist/lib/metadata/types/metadata-types'

export interface Config {
  metadata: {
    title: string
    icons: null | IconURL | Array<Icon> | Icons
    description: string
  }
}

const getConfigFile = async () => {
  const configPath = path.resolve('.', 'front-config.yml')
  return await readYamlFile<Config>(configPath)
}

export const getMetadata = async () => {
  const config: Config = await getConfigFile()

  return config.metadata
}
