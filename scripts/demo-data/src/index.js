#!/usr/bin/env node
/**
 * Midaz Demo Data Generator
 * CLI entry point for generating demo data
 */
import { Command } from 'commander';
import dotenv from 'dotenv';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { DataOrchestrator } from './orchestrator.js';
import logger from './utils/logger.js';

// Load environment variables
dotenv.config();

// Get package version from package.json
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const packageJsonPath = path.join(__dirname, '..', 'package.json');
const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));

// Create CLI program
const program = new Command();

program
  .name('midaz-demo-data')
  .description('Generate demo data for Midaz')
  .version(packageJson.version);

program
  .option('-v, --volume <size>', 'Volume of data to generate (small, medium, large)', 'small')
  .option('-u, --auth-token <token>', 'Auth token for API access')
  .option('-b, --base-url <url>', 'Base URL for API', 'http://localhost')
  .option('--onboarding-port <port>', 'Port for onboarding service', '3000')
  .option('--transaction-port <port>', 'Port for transaction service', '3001')
  .action(async (options) => {
    try {
      // Set environment variables from options
      if (options.authToken) {
        process.env.AUTH_TOKEN = options.authToken;
      }
      if (options.baseUrl) {
        process.env.API_BASE_URL = options.baseUrl;
      }
      if (options.onboardingPort) {
        process.env.ONBOARDING_PORT = options.onboardingPort;
      }
      if (options.transactionPort) {
        process.env.TRANSACTION_PORT = options.transactionPort;
      }
      
      // Validate volume size
      const validVolumes = ['small', 'medium', 'large'];
      if (!validVolumes.includes(options.volume)) {
        logger.error(`Invalid volume size: ${options.volume}. Must be one of: ${validVolumes.join(', ')}`);
        process.exit(1);
      }
      
      // Validate auth token
      if (!process.env.AUTH_TOKEN) {
        logger.warning('No auth token provided. API requests might fail if authentication is required.');
      }
      
      // Show configuration
      logger.info('Starting Midaz Demo Data Generator');
      logger.info(`Volume: ${options.volume}`);
      logger.info(`API URL: ${process.env.API_BASE_URL}`);
      logger.info(`Onboarding Port: ${process.env.ONBOARDING_PORT}`);
      logger.info(`Transaction Port: ${process.env.TRANSACTION_PORT}`);
      
      // Create orchestrator and run data generation
      const orchestrator = new DataOrchestrator(options.volume);
      const summary = await orchestrator.run();
      
      // Display summary
      logger.info('\nGeneration Summary:');
      logger.info(`- Organizations: ${summary.organizations}`);
      logger.info(`- Ledgers: ${summary.ledgers}`);
      logger.info(`- Assets: ${summary.assets}`);
      logger.info(`- Segments: ${summary.segments}`);
      logger.info(`- Portfolios: ${summary.portfolios}`);
      logger.info(`- Accounts: ${summary.accounts}`);
      logger.info(`- Transactions: ${summary.transactions}`);
      logger.info(`- Total entities: ${summary.totalEntities}`);
      
      logger.success('Demo data generation completed successfully!');
    } catch (error) {
      logger.error(`Failed to generate demo data: ${error.message}`);
      process.exit(1);
    }
  });

// Parse command line arguments
program.parse(process.argv);