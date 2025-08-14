#!/usr/bin/env node

/**
 * MarkdownParser Class
 * 
 * Parses workflow steps from Markdown format with robust error handling
 * and validation. Maintains the exact parsing logic from the original
 * implementation while adding improved error reporting and validation.
 */

class ParseError extends Error {
  constructor(message, context = {}) {
    super(message);
    this.name = 'ParseError';
    this.context = context;
  }
}

class ValidationError extends Error {
  constructor(message, issues = []) {
    super(message);
    this.name = 'ValidationError';
    this.issues = issues;
  }
}

class MarkdownParser {
  constructor(config) {
    this.config = config;
    this.patterns = {
      stepTitle: /^(\d+)\.\s+\*\*(.+)\*\*/,
      endpoint: /- `([A-Z]+)\s+([^`]+)`/,
      uses: /\*\*Uses:\*\*\s*(.+)/,
      output: /\*\*Outputs?:\*\*\s*(.+)/,
      variable: /`([^`]+)`/g,
      usePattern: /`([^`]+)`\s+from\s+step\s+(\d+)/g,
      outputPattern: /`([^`]+)`/g,
      description: /^- (.+)/,
    };
  }

  /**
   * Parse workflow steps from markdown content
   * @param {string} markdown - The markdown content to parse
   * @returns {Array} Array of workflow step objects
   */
  parse(markdown) {
    const lines = markdown.split('\n');
    const steps = [];
    let currentStep = null;
    let lineNumber = 0;

    console.log('Starting markdown parsing...');

    for (const line of lines) {
      lineNumber++;

      try {
        if (this.isStepTitle(line)) {
          currentStep = this.parseStepTitle(line);
          steps.push(currentStep);
          
          // Debug logging for special steps
          if (currentStep.title === "Zero Out Balance") {
            console.log(`DEBUG: Found Zero Out Balance step in markdown: number=${currentStep.number}, title="${currentStep.title}"`);
          }
        } else if (currentStep) {
          this.parseLine(line, currentStep);
        }
      } catch (error) {
        throw new ParseError(
          `Error parsing line ${lineNumber}: ${error.message}`,
          { line, lineNumber, step: currentStep }
        );
      }
    }

    console.log(`Parsed ${steps.length} workflow steps from Markdown.`);
    return this.validate(steps);
  }

  /**
   * Check if a line contains a step title
   * @param {string} line - Line to check
   * @returns {boolean}
   */
  isStepTitle(line) {
    return this.patterns.stepTitle.test(line.trim());
  }

  /**
   * Parse a step title line
   * @param {string} line - Line containing step title
   * @returns {Object} Step object with basic properties
   */
  parseStepTitle(line) {
    const match = line.trim().match(this.patterns.stepTitle);
    if (!match) {
      throw new Error('Invalid step title format');
    }

    const stepNumber = parseInt(match[1], 10);
    const title = match[2].trim();

    return {
      number: stepNumber,
      title: title,
      method: '',
      path: '',
      description: '',
      uses: [],
      outputs: []
    };
  }

  /**
   * Parse a line and update the current step object
   * @param {string} line - Line to parse
   * @param {Object} currentStep - Current step object to update
   */
  parseLine(line, currentStep) {
    const trimmedLine = line.trim();

    // Match method and path: "- `POST /v1/organizations`"
    if (trimmedLine.startsWith('- `')) {
      const endpointMatch = trimmedLine.match(this.patterns.endpoint);
      if (endpointMatch) {
        currentStep.method = endpointMatch[1];
        currentStep.path = endpointMatch[2];
        return;
      }
    }

    // Match description: "- Creates a new organization..." (ensure it's not Uses/Output)
    if (trimmedLine.startsWith('- ') && 
        !trimmedLine.includes('**Uses:**') && 
        !trimmedLine.includes('**Output')) {
      currentStep.description = trimmedLine.substring(2).trim();
      return;
    }

    // Match Uses section
    if (trimmedLine.includes('**Uses:**')) {
      this.parseUses(trimmedLine, currentStep);
      return;
    }

    // Match Output section
    if (trimmedLine.includes('**Output:**') || trimmedLine.includes('**Outputs:**')) {
      this.parseOutputs(trimmedLine, currentStep);
      return;
    }
  }

  /**
   * Parse Uses section
   * @param {string} line - Line containing Uses
   * @param {Object} currentStep - Current step object
   */
  parseUses(line, currentStep) {
    // Check if uses is on the same line - handle multiple uses separated by commas
    const inlineUsesMatch = line.match(this.patterns.uses);
    if (inlineUsesMatch) {
      const usesText = inlineUsesMatch[1];
      // Match all `variable` from step X patterns and extract them
      const useMatches = [...usesText.matchAll(this.patterns.usePattern)];
      
      for (const match of useMatches) {
        currentStep.uses.push({ 
          variable: match[1], 
          step: parseInt(match[2], 10) 
        });
      }
    }
  }

  /**
   * Parse Outputs section
   * @param {string} line - Line containing Outputs
   * @param {Object} currentStep - Current step object
   */
  parseOutputs(line, currentStep) {
    // Check if output is on the same line - handle multiple outputs separated by commas
    const inlineOutputMatch = line.match(this.patterns.output);
    if (inlineOutputMatch) {
      const outputsText = inlineOutputMatch[1];
      // Match all `variable` patterns and extract them
      const outputMatches = [...outputsText.matchAll(this.patterns.outputPattern)];
      
      for (const match of outputMatches) {
        const variable = match[1];
        currentStep.outputs.push(variable);
      }
    }
  }

  /**
   * Validate parsed steps
   * @param {Array} steps - Array of parsed steps
   * @returns {Array} Validated steps array
   */
  validate(steps) {
    const issues = [];

    // Ensure all required fields are present
    for (const step of steps) {
      if (!step.method || !step.path) {
        issues.push({
          type: 'MISSING_REQUIRED_FIELDS',
          step: step.number,
          title: step.title,
          message: `Step ${step.number} "${step.title}" missing method or path`,
          missing: {
            method: !step.method,
            path: !step.path
          }
        });
      }

      // Validate HTTP methods
      const validMethods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD'];
      if (step.method && !validMethods.includes(step.method)) {
        issues.push({
          type: 'INVALID_HTTP_METHOD',
          step: step.number,
          title: step.title,
          message: `Step ${step.number} has invalid HTTP method: ${step.method}`,
          method: step.method,
          validMethods
        });
      }

      // Validate path format
      if (step.path && !step.path.startsWith('/')) {
        issues.push({
          type: 'INVALID_PATH_FORMAT',
          step: step.number,
          title: step.title,
          message: `Step ${step.number} path should start with '/': ${step.path}`,
          path: step.path
        });
      }
    }

    // Validate step numbers are sequential
    for (let i = 0; i < steps.length; i++) {
      if (steps[i].number !== i + 1) {
        issues.push({
          type: 'NON_SEQUENTIAL_STEPS',
          expected: i + 1,
          actual: steps[i].number,
          message: `Step numbers not sequential at position ${i + 1}. Expected ${i + 1}, got ${steps[i].number}`
        });
      }
    }

    // Validate dependency references
    this.validateDependencies(steps, issues);

    if (issues.length > 0) {
      throw new ValidationError('Markdown validation failed', issues);
    }

    return steps;
  }

  /**
   * Validate step dependencies
   * @param {Array} steps - Array of steps
   * @param {Array} issues - Array to collect validation issues
   */
  validateDependencies(steps, issues) {
    const availableOutputs = new Map(); // stepNumber -> [outputs]
    
    for (const step of steps) {
      // Track outputs for this step
      if (step.outputs.length > 0) {
        availableOutputs.set(step.number, step.outputs);
      }

      // Validate that all dependencies are available from previous steps
      for (const dependency of step.uses) {
        let found = false;
        
        // Check if the referenced step exists and provides this variable
        for (let prevStepNum = 1; prevStepNum < step.number; prevStepNum++) {
          const prevOutputs = availableOutputs.get(prevStepNum);
          if (prevOutputs && prevOutputs.includes(dependency.variable)) {
            found = true;
            break;
          }
        }

        if (!found) {
          issues.push({
            type: 'UNRESOLVED_DEPENDENCY',
            step: step.number,
            title: step.title,
            dependency: dependency.variable,
            referencedStep: dependency.step,
            message: `Step ${step.number} requires "${dependency.variable}" from step ${dependency.step}, but no previous step provides this variable`
          });
        }

        // Check if dependency references a future step (forward dependency)
        if (dependency.step >= step.number) {
          issues.push({
            type: 'FORWARD_DEPENDENCY',
            step: step.number,
            title: step.title,
            dependency: dependency.variable,
            referencedStep: dependency.step,
            message: `Step ${step.number} cannot depend on future step ${dependency.step}`
          });
        }
      }
    }
  }

  /**
   * Get parsing statistics
   * @param {Array} steps - Array of parsed steps
   * @returns {Object} Statistics object
   */
  getStatistics(steps) {
    const stats = {
      totalSteps: steps.length,
      stepsWithDependencies: 0,
      stepsWithOutputs: 0,
      totalDependencies: 0,
      totalOutputs: 0,
      methodBreakdown: {},
      specialSteps: []
    };

    for (const step of steps) {
      if (step.uses.length > 0) {
        stats.stepsWithDependencies++;
        stats.totalDependencies += step.uses.length;
      }

      if (step.outputs.length > 0) {
        stats.stepsWithOutputs++;
        stats.totalOutputs += step.outputs.length;
      }

      // Count methods
      if (!stats.methodBreakdown[step.method]) {
        stats.methodBreakdown[step.method] = 0;
      }
      stats.methodBreakdown[step.method]++;

      // Track special steps
      if (this.config.stepTypes.SPECIAL.titles.includes(step.title)) {
        stats.specialSteps.push({
          number: step.number,
          title: step.title
        });
      }
    }

    return stats;
  }
}

module.exports = { MarkdownParser, ParseError, ValidationError };