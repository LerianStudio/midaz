/**
 * @formatjs/cli-lib has a breaking change in version 7.0.0
 * which requires the use of `lib_esnext` instead of `lib`
 *
 * https://github.com/formatjs/formatjs/issues/4764
 */
import { extract } from '@formatjs/cli-lib/lib_esnext'
import { glob } from 'glob'
import { intlConfig } from '../intl.config'
import { mkdir, open, readFile, writeFile } from 'fs/promises'
import path from 'path'
import { existsSync } from 'fs'
import { omit, mapValues } from 'lodash'

const outputPath = './locales/extracted'
const formatJsConfig = {
  format: 'simple',
  additionalFunctionNames: ['$t'],
  throws: true
}

/**
 * Analyses the differences in keys of an object A in relation to B
 * and returns the added and removed key values as new objects
 * @param a
 * @param b
 * @returns
 */
function diff(a: object, b: object) {
  return {
    added: omit(a, Object.keys(b)),
    removed: omit(b, Object.keys(a))
  }
}

/**
 * Apply the differences between an object A in relation to B
 * adding or removing those keys from object B
 * @param a Base line object
 * @param b Target object
 * @returns
 */
function applyDiff(a: object, b: object) {
  const { added, removed } = diff(a, b)

  return Object.assign({}, omit(b, Object.keys(removed)), added)
}

/**
 * Clears default messages from a extracted keys JSON string
 * @param data JSON string data
 * @returns
 */
function clearDefaultMessages(data: string) {
  let json = JSON.parse(data)

  const temp = mapValues(json, () => '')

  return JSON.stringify(temp, null, 2)
}

/**
 * Checks if a locale json file already exists
 * If exists, merge the already existing data with new data from arguments
 * and save the file
 * @param locale
 * @param data JSON string data
 */
async function extractLocale(locale: string, data: string) {
  let output = data
  const localePath = path.join(outputPath, `${locale}.json`)

  // Checks if file exists
  if (existsSync(localePath)) {
    let outputJson = JSON.parse(output)

    // Reads file contents
    const localeFile = await readFile(localePath)
    const localeJson = JSON.parse(localeFile.toString('utf-8'))

    // Merge existing keys with new empty keys, given preference to existing ones
    outputJson = applyDiff(outputJson, localeJson)

    output = JSON.stringify(outputJson, null, 2)
  }

  // Output into a file
  try {
    const fd = await open(localePath, 'w')

    await writeFile(fd, output, 'utf-8')
  } catch (e) {
    console.error(`Error writing file ${localePath}: ${(e as Error).message}`)
  }
}

async function main() {
  // Get the list of files
  const paths = await glob('./src/**/!(*.d).{js,ts,tsx}')

  // Creates output path in case it doesn't exists
  await mkdir(outputPath, { recursive: true })

  // Runs formatjs and get the extracted keys
  const extracted = await extract(paths, formatJsConfig)

  // Outputs default language file
  await writeFile(
    path.join(outputPath, `${intlConfig.defaultLocale}.json`),
    extracted,
    'utf-8'
  )

  // Remove default messages
  const extractedClean = clearDefaultMessages(extracted)

  // Outputs json files for each locale
  intlConfig.locales.map(async (locale) => {
    // Skips default locale
    if (locale === intlConfig.defaultLocale) {
      return
    }

    // Diff data and outputs each locale in a new file
    await extractLocale(locale, extractedClean)
  })
}

main()
