#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

/**
 * Expands environment variables in .env file
 * Converts ${VAR_NAME} references to actual values from process.env
 */
function expandEnvFile() {
  const envPath = path.join(process.cwd(), '.env');
  
  if (!fs.existsSync(envPath)) {
    console.log('No .env file found to expand');
    return;
  }

  let content = fs.readFileSync(envPath, 'utf8');
  
  // First pass: collect all defined variables in the file
  const definedVars = {};
  const lines = content.split('\n');
  
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed && !trimmed.startsWith('#')) {
      const [key, ...valueParts] = trimmed.split('=');
      if (key && valueParts.length > 0) {
        let value = valueParts.join('=');
        // Remove quotes if present
        if ((value.startsWith("'") && value.endsWith("'")) || 
            (value.startsWith('"') && value.endsWith('"'))) {
          value = value.slice(1, -1);
        }
        definedVars[key.trim()] = value;
      }
    }
  }

  // Second pass: expand variables
  let expanded = content;
  let changed = false;
  
  // Keep expanding until no more changes (handles nested references)
  let maxIterations = 10;
  while (maxIterations-- > 0) {
    const before = expanded;
    
    // Replace ${VAR_NAME} with actual values
    expanded = expanded.replace(/\$\{([^}]+)\}/g, (match, varName) => {
      const value = process.env[varName] || definedVars[varName];
      if (value !== undefined) {
        changed = true;
        return value;
      }
      return match; // Keep unchanged if variable not found
    });
    
    if (expanded === before) {
      break; // No more changes
    }
  }

  if (changed) {
    fs.writeFileSync(envPath, expanded);
    console.log('Environment variables expanded in .env file');
  } else {
    console.log('No environment variables to expand');
  }
}

expandEnvFile();