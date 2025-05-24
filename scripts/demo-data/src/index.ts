#!/usr/bin/env node
/**
 * Midaz Demo Data Generator
 * CLI entry point
 */

// Import yargs with proper typing
import yargs, { Argv } from 'yargs';
import { hideBin } from 'yargs/helpers';

// Handle Faker import differently
// eslint-disable-next-line @typescript-eslint/no-var-requires
const faker = require('faker');

// Initialize faker with pt_BR locale
// This works across different versions of faker
require('faker/locale/pt_BR');
import { Generator } from './generator';
import { DEFAULT_OPTIONS } from './config';
import { VolumeSize } from './types';

async function main() {
  // Properly typed yargs instance
  const argv = await yargs(hideBin(process.argv))
    .options({
      volume: {
        alias: 'v',
        describe: 'Data volume size',
        choices: Object.values(VolumeSize),
        default: DEFAULT_OPTIONS.volume,
      },
      'base-url': {
        alias: 'u',
        describe: 'API base URL',
        type: 'string',
        default: DEFAULT_OPTIONS.baseUrl,
      },
      'onboarding-port': {
        alias: 'o',
        describe: 'Onboarding service port',
        type: 'number',
        default: DEFAULT_OPTIONS.onboardingPort,
      },
      'transaction-port': {
        alias: 't',
        describe: 'Transaction service port',
        type: 'number',
        default: DEFAULT_OPTIONS.transactionPort,
      },
      concurrency: {
        alias: 'c',
        describe: 'Concurrency level',
        type: 'number',
        default: DEFAULT_OPTIONS.concurrency,
      },
      debug: {
        alias: 'd',
        describe: 'Enable debug mode',
        type: 'boolean',
        default: DEFAULT_OPTIONS.debug,
      },
      'auth-token': {
        alias: 'a',
        describe: 'Authentication token',
        type: 'string',
      },
      seed: {
        alias: 's',
        describe: 'Random seed for reproducible runs',
        type: 'number',
      },
    })
    .usage('Usage: $0 [options]')
    .example('$0 --volume small', 'Generate a small amount of demo data')
    .example('$0 --volume large --debug', 'Generate a large amount of demo data with debug logging')
    .example(
      '$0 --base-url http://localhost --onboarding-port 3000',
      'Connect to specific API endpoints'
    )
    .epilogue('For more information, check the README.md file')
    .help()
    .alias('help', 'h').argv;

  // Configure faker with seed if provided
  if (argv.seed !== undefined) {
    console.log(`Using random seed: ${argv.seed}`);
    faker.seed(argv.seed);
  } else {
    // Use current timestamp as seed for more randomness
    const seed = Date.now();
    console.log(`Using dynamic seed: ${seed}`);
    // Use seedValue method for v5.5.x of faker
    faker.seedValue = seed;
  }

  // Use Brazilian Portuguese locale
  // We need to import Brazilian locale data directly
  console.log('Using locale: pt_BR');

  // Configure options for generator
  const options = {
    volume: argv.volume as VolumeSize,
    baseUrl: argv['base-url'],
    onboardingPort: argv['onboarding-port'],
    transactionPort: argv['transaction-port'],
    concurrency: argv.concurrency,
    debug: argv.debug,
    authToken: argv['auth-token'],
    seed: argv.seed,
  };

  console.log(`Starting Midaz demo data generator with volume: ${options.volume}`);
  console.log(
    `Connecting to ${options.baseUrl}:${options.onboardingPort} and ${options.baseUrl}:${options.transactionPort}`
  );

  try {
    // Run the generator
    const generator = new Generator(options);
    await generator.run();

    console.log('Demo data generation completed successfully!');
    process.exit(0);
  } catch (error) {
    console.error('Error during demo data generation:', error);
    process.exit(1);
  }
}

// Execute main function
main().catch((error) => {
  console.error('Unexpected error:', error);
  process.exit(1);
});
