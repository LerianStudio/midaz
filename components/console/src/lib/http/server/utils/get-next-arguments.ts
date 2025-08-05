export function getNextRequestArgument(args: any[]) {
  if (!args) {
    return undefined
  }

  return args[0]
}

export function getNextParamArgument(args: any[]) {
  if (!args) {
    return undefined
  }

  if (!args[1]?.params) {
    return undefined
  }

  return args[1].params
}
