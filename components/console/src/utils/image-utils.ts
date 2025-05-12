export const encodeBase64 = (file: File): string => {
  const reader = new FileReader()
  reader.readAsDataURL(file)

  return reader.result as string
}
