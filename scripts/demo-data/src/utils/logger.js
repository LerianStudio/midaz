/**
 * Logging utility for the demo data generator
 */
import ora from 'ora';

// Spinners for visual feedback
const spinners = {};

/**
 * Start a spinner with a message
 * @param {string} id - Unique identifier for the spinner
 * @param {string} text - Text to display
 * @returns {Object} - Ora spinner instance
 */
export const startSpinner = (id, text) => {
  if (spinners[id]) {
    spinners[id].text = text;
    spinners[id].start();
  } else {
    spinners[id] = ora(text).start();
  }
  return spinners[id];
};

/**
 * Update a spinner's text
 * @param {string} id - Spinner identifier
 * @param {string} text - New text to display
 */
export const updateSpinner = (id, text) => {
  if (spinners[id]) {
    spinners[id].text = text;
  } else {
    spinners[id] = ora(text).start();
  }
};

/**
 * Complete a spinner with success
 * @param {string} id - Spinner identifier
 * @param {string} text - Completion text
 */
export const succeedSpinner = (id, text) => {
  if (spinners[id]) {
    // Add a small delay to ensure the spinner isn't immediately replaced
    setTimeout(() => {
      spinners[id].succeed(text);
      delete spinners[id];
    }, 100);
  }
};

/**
 * Complete a spinner with failure
 * @param {string} id - Spinner identifier
 * @param {string} text - Failure text
 */
export const failSpinner = (id, text) => {
  if (spinners[id]) {
    // Add a small delay to ensure the spinner isn't immediately replaced
    setTimeout(() => {
      spinners[id].fail(text);
      delete spinners[id];
    }, 100);
  }
};

/**
 * Log an info message
 * @param {string} message - Message to log
 */
export const info = (message) => {
  console.log(`\n ℹ️  ${message}`);
};

/**
 * Log a success message
 * @param {string} message - Message to log
 */
export const success = (message) => {
  console.log(`\n ✅ ${message}`);
};

/**
 * Log a warning message
 * @param {string} message - Message to log
 */
export const warning = (message) => {
  console.log(`\n ⚠️  ${message}`);
};

/**
 * Log an error message
 * @param {string} message - Message to log
 */
export const error = (message) => {
  console.error(`\n ❌ ${message}`);
};

export default {
  startSpinner,
  updateSpinner,
  succeedSpinner,
  failSpinner,
  info,
  success,
  warning,
  error
};