/**
 * Validates if the SVG file contains any script tags or onload attributes
 * that is considered dangerous or malicious.
 *
 * @param file SVG file as text
 * @returns
 */
export function validateSVG(file: string) {
  return !['script', 'onload'].some((tag) => file.includes(tag))
}
