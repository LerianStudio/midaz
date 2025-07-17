export type DockerTagDto = {
  id: number
  name: string
  last_updated: string
  tag_status: 'active' | 'inactive'
}

export type DockerListRepositoryTagsDto = {
  count: number
  next: string | null
  previous: string | null
  results: DockerTagDto[]
}
