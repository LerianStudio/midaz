/**
 * Downloads a file from a URL as text data.
 * @param url The file URL
 * @returns The file data as text
 */
export async function downloadFile(url: string) {
  return await fetch(url).then((response) => response.text())
}
