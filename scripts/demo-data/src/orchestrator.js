/**
 * Orchestrator for demo data generation process
 * Handles creation of entities in the correct order and manages dependencies
 */
import pLimit from 'p-limit';
import config from '../config.js';
import logger from './utils/logger.js';
import api from './api.js';

// Import generators
import organizationGenerator from './generators/organization-generator.js';
import ledgerGenerator from './generators/ledger-generator.js';
import assetGenerator from './generators/asset-generator.js';
import segmentGenerator from './generators/segment-generator.js';
import portfolioGenerator from './generators/portfolio-generator.js';
import accountGenerator from './generators/account-generator.js';
import transactionGenerator from './generators/transaction-generator.js';

/**
 * Main orchestrator class for data generation
 */
export class DataOrchestrator {
  constructor(volumeSize = 'small') {
    this.volumeSize = volumeSize;
    this.volumes = config.volumes[volumeSize];
    this.entities = {
      organizations: [],
      ledgers: [],
      assets: [],
      segments: [],
      portfolios: [],
      accounts: [],
      transactions: []
    };
    this.limiters = {
      organizations: pLimit(config.concurrency.organizations),
      ledgers: pLimit(config.concurrency.ledgers),
      assets: pLimit(config.concurrency.assets),
      segments: pLimit(config.concurrency.segments),
      portfolios: pLimit(config.concurrency.portfolios),
      accounts: pLimit(config.concurrency.accounts),
      transactions: pLimit(config.concurrency.transactions)
    };
  }

  /**
   * Run the entire data generation process
   */
  async run() {
    try {
      logger.info(`Starting demo data generation (${this.volumeSize} volume)`);
      logger.info(`This process may take several minutes to complete. Please be patient.`);
      
      const startTime = Date.now();
      
      const steps = [
        { name: 'Organizations', function: this.createOrganizations.bind(this) },
        { name: 'Ledgers', function: this.createLedgers.bind(this) },
        { name: 'Assets', function: this.createAssets.bind(this) },
        { name: 'Segments', function: this.createSegments.bind(this) },
        { name: 'Portfolios', function: this.createPortfolios.bind(this) },
        { name: 'Accounts', function: this.createAccounts.bind(this) },
        { name: 'Initial Deposits', function: this.createInitialDeposits.bind(this) },
        { name: 'Transactions', function: this.createTransactions.bind(this) }
      ];
      
      let completedSteps = 0;
      const totalSteps = steps.length;
      
      // Create and update a progress spinner
      logger.startSpinner('progress', `Overall progress: ${completedSteps}/${totalSteps} steps completed (0%)`);
      
      for (const step of steps) {
        logger.updateSpinner('progress', `Overall progress: ${completedSteps}/${totalSteps} steps completed (${Math.round(completedSteps/totalSteps*100)}%) - Working on ${step.name}...`);
        await step.function();
        completedSteps++;
        logger.updateSpinner('progress', `Overall progress: ${completedSteps}/${totalSteps} steps completed (${Math.round(completedSteps/totalSteps*100)}%)`);
      }
      
      const totalTime = ((Date.now() - startTime) / 1000).toFixed(2);
      logger.succeedSpinner('progress', `Demo data generation completed successfully in ${totalTime} seconds`);
      
      return this.generateSummary();
    } catch (error) {
      logger.failSpinner('progress', `Error in data generation process: ${error.message}`);
      logger.error(`Error in data generation process: ${error.message}`);
      throw error;
    }
  }

  /**
   * Create organizations
   */
  async createOrganizations() {
    const count = this.volumes.organizations;
    logger.startSpinner('orgs', `Generating ${count} organizations...`);
    
    try {
      const organizations = organizationGenerator.generateOrganizations(count);
      
      // Create organizations in parallel with rate limiting
      const promises = organizations.map((orgData, index) => {
        return this.limiters.organizations(async () => {
          logger.updateSpinner('orgs', `Creating organization ${index + 1}/${count}...`);
          const org = await api.organizationAPI.create(orgData);
          return org;
        });
      });
      
      this.entities.organizations = await Promise.all(promises);
      logger.succeedSpinner('orgs', `Created ${count} organizations`);
    } catch (error) {
      logger.failSpinner('orgs', `Failed to create organizations: ${error.message}`);
      throw error;
    }
  }

  /**
   * Create ledgers for each organization
   */
  async createLedgers() {
    if (this.entities.organizations.length === 0) {
      logger.warning('No organizations found. Skipping ledger creation.');
      return;
    }
    
    const ledgersPerOrg = this.volumes.ledgersPerOrg;
    const totalLedgers = this.entities.organizations.length * ledgersPerOrg;
    logger.startSpinner('ledgers', `Generating ${totalLedgers} ledgers...`);
    
    try {
      const promises = [];
      
      for (const org of this.entities.organizations) {
        const ledgers = ledgerGenerator.generateLedgers(org, ledgersPerOrg);
        
        for (const [index, ledgerData] of ledgers.entries()) {
          promises.push(
            this.limiters.ledgers(async () => {
              logger.updateSpinner('ledgers', `Creating ledger ${index + 1}/${ledgersPerOrg} for org ${org.id}...`);
              const ledger = await api.ledgerAPI.create(org.id, ledgerData);
              ledger.organizationId = org.id; // Store reference to parent org
              return ledger;
            })
          );
        }
      }
      
      this.entities.ledgers = await Promise.all(promises);
      logger.succeedSpinner('ledgers', `Created ${this.entities.ledgers.length} ledgers`);
    } catch (error) {
      logger.failSpinner('ledgers', `Failed to create ledgers: ${error.message}`);
      throw error;
    }
  }

  /**
   * Create assets for each ledger
   */
  async createAssets() {
    if (this.entities.ledgers.length === 0) {
      logger.warning('No ledgers found. Skipping asset creation.');
      return;
    }
    
    const assetsPerLedger = this.volumes.assetsPerLedger;
    const totalAssets = this.entities.ledgers.length * assetsPerLedger;
    logger.startSpinner('assets', `Generating ${totalAssets} assets...`);
    
    try {
      const promises = [];
      const assetCodes = config.random.assetCodes.slice(0, assetsPerLedger);
      
      for (const ledger of this.entities.ledgers) {
        const assets = assetGenerator.generateAssets(assetCodes);
        
        for (const [index, assetData] of assets.entries()) {
          promises.push(
            this.limiters.assets(async () => {
              logger.updateSpinner('assets', `Creating ${assetData.code} asset for ledger ${ledger.id}...`);
              const asset = await api.assetAPI.create(ledger.organizationId, ledger.id, assetData);
              asset.ledgerId = ledger.id; // Store reference to parent ledger
              asset.organizationId = ledger.organizationId; // Store reference to parent org
              return asset;
            })
          );
        }
      }
      
      this.entities.assets = await Promise.all(promises);
      logger.succeedSpinner('assets', `Created ${this.entities.assets.length} assets`);
    } catch (error) {
      logger.failSpinner('assets', `Failed to create assets: ${error.message}`);
      throw error;
    }
  }

  /**
   * Create segments for each ledger
   */
  async createSegments() {
    if (this.entities.ledgers.length === 0) {
      logger.warning('No ledgers found. Skipping segment creation.');
      return;
    }
    
    const segmentsPerLedger = this.volumes.segmentsPerLedger;
    const totalSegments = this.entities.ledgers.length * segmentsPerLedger;
    logger.startSpinner('segments', `Generating ${totalSegments} segments...`);
    
    try {
      const promises = [];
      
      for (const ledger of this.entities.ledgers) {
        const segments = segmentGenerator.generateSegments(segmentsPerLedger);
        
        for (const [index, segmentData] of segments.entries()) {
          promises.push(
            this.limiters.segments(async () => {
              logger.updateSpinner('segments', `Creating segment ${index + 1}/${segmentsPerLedger} for ledger ${ledger.id}...`);
              const segment = await api.segmentAPI.create(ledger.organizationId, ledger.id, segmentData);
              segment.ledgerId = ledger.id; // Store reference to parent ledger
              segment.organizationId = ledger.organizationId; // Store reference to parent org
              return segment;
            })
          );
        }
      }
      
      this.entities.segments = await Promise.all(promises);
      logger.succeedSpinner('segments', `Created ${this.entities.segments.length} segments`);
    } catch (error) {
      logger.failSpinner('segments', `Failed to create segments: ${error.message}`);
      throw error;
    }
  }

  /**
   * Create portfolios for each ledger
   */
  async createPortfolios() {
    if (this.entities.ledgers.length === 0) {
      logger.warning('No ledgers found. Skipping portfolio creation.');
      return;
    }
    
    const portfoliosPerLedger = this.volumes.portfoliosPerLedger;
    const totalPortfolios = this.entities.ledgers.length * portfoliosPerLedger;
    logger.startSpinner('portfolios', `Generating ${totalPortfolios} portfolios...`);
    
    try {
      const promises = [];
      
      for (const ledger of this.entities.ledgers) {
        const portfolios = portfolioGenerator.generatePortfolios(portfoliosPerLedger);
        
        for (const [index, portfolioData] of portfolios.entries()) {
          promises.push(
            this.limiters.portfolios(async () => {
              logger.updateSpinner('portfolios', `Creating portfolio ${index + 1}/${portfoliosPerLedger} for ledger ${ledger.id}...`);
              const portfolio = await api.portfolioAPI.create(ledger.organizationId, ledger.id, portfolioData);
              portfolio.ledgerId = ledger.id; // Store reference to parent ledger
              portfolio.organizationId = ledger.organizationId; // Store reference to parent org
              return portfolio;
            })
          );
        }
      }
      
      this.entities.portfolios = await Promise.all(promises);
      logger.succeedSpinner('portfolios', `Created ${this.entities.portfolios.length} portfolios`);
    } catch (error) {
      logger.failSpinner('portfolios', `Failed to create portfolios: ${error.message}`);
      throw error;
    }
  }

  /**
   * Create accounts for each ledger
   */
  async createAccounts() {
    if (this.entities.ledgers.length === 0) {
      logger.warning('No ledgers found. Skipping account creation.');
      return;
    }
    
    const accountsPerLedger = this.volumes.accountsPerLedger;
    const totalAccounts = this.entities.ledgers.length * accountsPerLedger;
    logger.startSpinner('accounts', `Generating ${totalAccounts} accounts...`);
    
    try {
      const promises = [];
      
      for (const ledger of this.entities.ledgers) {
        // Get segments for this ledger
        const ledgerSegments = this.entities.segments.filter(s => s.ledgerId === ledger.id);
        
        // Get portfolios for this ledger
        const ledgerPortfolios = this.entities.portfolios.filter(p => p.ledgerId === ledger.id);
        
        // Generate accounts
        const accounts = accountGenerator.generateAccounts(ledgerSegments, ledgerPortfolios, accountsPerLedger);
        
        for (const [index, accountData] of accounts.entries()) {
          promises.push(
            this.limiters.accounts(async () => {
              logger.updateSpinner('accounts', `Creating account ${index + 1}/${accountsPerLedger} for ledger ${ledger.id}...`);
              const account = await api.accountAPI.create(ledger.organizationId, ledger.id, accountData);
              account.ledgerId = ledger.id; // Store reference to parent ledger
              account.organizationId = ledger.organizationId; // Store reference to parent org
              account.alias = accountData.alias; // Keep the alias for easy reference
              return account;
            })
          );
        }
      }
      
      this.entities.accounts = await Promise.all(promises);
      logger.succeedSpinner('accounts', `Created ${this.entities.accounts.length} accounts`);
    } catch (error) {
      logger.failSpinner('accounts', `Failed to create accounts: ${error.message}`);
      throw error;
    }
  }

  /**
   * Create initial deposits for each account
   */
  async createInitialDeposits() {
    if (this.entities.accounts.length === 0) {
      logger.warning('No accounts found. Skipping initial deposit creation.');
      return;
    }
    
    logger.startSpinner('deposits', `Creating initial deposits for ${this.entities.accounts.length} accounts...`);
    
    try {
      const promises = [];
      
      // Group accounts by ledger for efficient processing
      const accountsByLedger = this.entities.accounts.reduce((acc, account) => {
        if (!acc[account.ledgerId]) {
          acc[account.ledgerId] = [];
        }
        acc[account.ledgerId].push(account);
        return acc;
      }, {});
      
      // For each ledger, create initial deposits for its accounts
      for (const [ledgerId, accounts] of Object.entries(accountsByLedger)) {
        // Find the ledger
        const ledger = this.entities.ledgers.find(l => l.id === ledgerId);
        if (!ledger) continue;
        
        // Find BRL asset for this ledger
        const brlAsset = this.entities.assets.find(a => a.ledgerId === ledgerId && a.code === 'BRL');
        if (!brlAsset) continue;
        
        // Generate and create initial deposit transactions
        const deposits = transactionGenerator.generateInitialDeposits(accounts, 'BRL');
        
        for (const [index, depositData] of deposits.entries()) {
          promises.push(
            this.limiters.transactions(async () => {
              logger.updateSpinner('deposits', `Creating initial deposit ${index + 1}/${deposits.length} for ledger ${ledgerId}...`);
              const transaction = await api.transactionAPI.create(ledger.organizationId, ledgerId, depositData);
              transaction.ledgerId = ledgerId; // Store reference to parent ledger
              transaction.organizationId = ledger.organizationId; // Store reference to parent org
              return transaction;
            })
          );
        }
      }
      
      const initialDeposits = await Promise.all(promises);
      this.entities.transactions = [...this.entities.transactions, ...initialDeposits];
      logger.succeedSpinner('deposits', `Created ${initialDeposits.length} initial deposits`);
    } catch (error) {
      logger.failSpinner('deposits', `Failed to create initial deposits: ${error.message}`);
      throw error;
    }
  }

  /**
   * Create transactions between accounts
   */
  async createTransactions() {
    if (this.entities.accounts.length === 0) {
      logger.warning('No accounts found. Skipping transaction creation.');
      return;
    }
    
    const transactionsPerAccount = this.volumes.transactionsPerAccount;
    const totalTransactions = this.entities.accounts.length * transactionsPerAccount;
    logger.startSpinner('transactions', `Generating ${totalTransactions} transactions...`);
    
    try {
      const promises = [];
      
      // Group accounts by ledger for efficient processing
      const accountsByLedger = this.entities.accounts.reduce((acc, account) => {
        if (!acc[account.ledgerId]) {
          acc[account.ledgerId] = [];
        }
        acc[account.ledgerId].push(account);
        return acc;
      }, {});
      
      // For each ledger with at least 2 accounts, create transactions
      for (const [ledgerId, accounts] of Object.entries(accountsByLedger)) {
        if (accounts.length < 2) {
          logger.warning(`Ledger ${ledgerId} has less than 2 accounts. Skipping transaction creation.`);
          continue;
        }
        
        // Find the ledger
        const ledger = this.entities.ledgers.find(l => l.id === ledgerId);
        if (!ledger) continue;
        
        // Generate and create transactions
        const transactions = transactionGenerator.generateTransactions(accounts, 'BRL', transactionsPerAccount);
        
        for (const [index, transactionData] of transactions.entries()) {
          promises.push(
            this.limiters.transactions(async () => {
              logger.updateSpinner('transactions', `Creating transaction ${index + 1}/${transactions.length} for ledger ${ledgerId}...`);
              const transaction = await api.transactionAPI.create(ledger.organizationId, ledgerId, transactionData);
              transaction.ledgerId = ledgerId; // Store reference to parent ledger
              transaction.organizationId = ledger.organizationId; // Store reference to parent org
              return transaction;
            })
          );
        }
      }
      
      const accountTransactions = await Promise.all(promises);
      this.entities.transactions = [...this.entities.transactions, ...accountTransactions];
      logger.succeedSpinner('transactions', `Created ${accountTransactions.length} transactions`);
    } catch (error) {
      logger.failSpinner('transactions', `Failed to create transactions: ${error.message}`);
      throw error;
    }
  }

  /**
   * Generate a summary of created entities
   * @returns {Object} - Summary of created entities
   */
  generateSummary() {
    const summary = {
      organizations: this.entities.organizations.length,
      ledgers: this.entities.ledgers.length,
      assets: this.entities.assets.length,
      segments: this.entities.segments.length,
      portfolios: this.entities.portfolios.length,
      accounts: this.entities.accounts.length,
      transactions: this.entities.transactions.length,
      totalEntities: 
        this.entities.organizations.length +
        this.entities.ledgers.length +
        this.entities.assets.length +
        this.entities.segments.length +
        this.entities.portfolios.length +
        this.entities.accounts.length +
        this.entities.transactions.length
    };
    
    // Display the summary
    logger.info('Demo Data Generation Summary:');
    console.log('\n┌─────────────────────────────────┐');
    console.log('│  MIDAZ DEMO DATA GENERATION      │');
    console.log('├─────────────────────────────────┤');
    console.log(`│  Organizations:   ${summary.organizations.toString().padStart(5)}        │`);
    console.log(`│  Ledgers:         ${summary.ledgers.toString().padStart(5)}        │`);
    console.log(`│  Assets:          ${summary.assets.toString().padStart(5)}        │`);
    console.log(`│  Segments:        ${summary.segments.toString().padStart(5)}        │`);
    console.log(`│  Portfolios:      ${summary.portfolios.toString().padStart(5)}        │`);
    console.log(`│  Accounts:        ${summary.accounts.toString().padStart(5)}        │`);
    console.log(`│  Transactions:    ${summary.transactions.toString().padStart(5)}        │`);
    console.log('├─────────────────────────────────┤');
    console.log(`│  TOTAL ENTITIES:  ${summary.totalEntities.toString().padStart(5)}        │`);
    console.log('└─────────────────────────────────┘\n');
    
    return summary;
  }
}

export default DataOrchestrator;