#!/usr/bin/env node

/**
 * Swagger to OpenAPI YAML Converter
 * 
 * This script converts Swagger JSON to OpenAPI YAML format,
 * replacing the need for Docker-based openapi-generator-cli
 * 
 * Usage: node swagger-to-openapi.js <input.json> <output.yaml>
 */

const fs = require('fs');
const path = require('path');
const yaml = require('js-yaml');

// Parse command line arguments
const args = process.argv.slice(2);
if (args.length !== 2) {
    console.error('Usage: node swagger-to-openapi.js <input.json> <output.yaml>');
    process.exit(1);
}

const [inputFile, outputFile] = args;

// Function to clean up the OpenAPI spec
function cleanOpenAPISpec(spec) {
    // Remove unnecessary fields that openapi-generator adds
    const cleaned = JSON.parse(JSON.stringify(spec));
    
    // Remove x-go-package from schemas if present
    if (cleaned.components && cleaned.components.schemas) {
        Object.values(cleaned.components.schemas).forEach(schema => {
            delete schema['x-go-package'];
        });
    }
    
    // Ensure proper OpenAPI version
    if (!cleaned.openapi) {
        cleaned.openapi = '3.0.0';
    }
    
    // Clean up servers if needed
    if (!cleaned.servers || cleaned.servers.length === 0) {
        cleaned.servers = [
            {
                url: '/',
                description: 'Default server'
            }
        ];
    }
    
    return cleaned;
}

// Function to convert Swagger 2.0 to OpenAPI 3.0 if needed
function convertSwaggerToOpenAPI(swagger) {
    // If it's already OpenAPI 3.0, just clean and return
    if (swagger.openapi) {
        return cleanOpenAPISpec(swagger);
    }
    
    // Convert from Swagger 2.0 to OpenAPI 3.0
    const openapi = {
        openapi: '3.0.0',
        info: swagger.info,
        servers: swagger.servers || [
            {
                url: (swagger.schemes ? swagger.schemes[0] : 'https') + '://' + 
                     (swagger.host || 'localhost') + 
                     (swagger.basePath || ''),
                description: 'Default server'
            }
        ],
        paths: {},
        components: {
            schemas: swagger.definitions || {},
            securitySchemes: {}
        }
    };
    
    // Convert security definitions
    if (swagger.securityDefinitions) {
        Object.keys(swagger.securityDefinitions).forEach(key => {
            const def = swagger.securityDefinitions[key];
            if (def.type === 'basic') {
                openapi.components.securitySchemes[key] = {
                    type: 'http',
                    scheme: 'basic'
                };
            } else if (def.type === 'apiKey') {
                openapi.components.securitySchemes[key] = {
                    type: 'apiKey',
                    in: def.in,
                    name: def.name
                };
            } else if (def.type === 'oauth2') {
                openapi.components.securitySchemes[key] = {
                    type: 'oauth2',
                    flows: {}
                };
                // Convert OAuth2 flows
                if (def.flow === 'implicit') {
                    openapi.components.securitySchemes[key].flows.implicit = {
                        authorizationUrl: def.authorizationUrl,
                        scopes: def.scopes || {}
                    };
                }
            }
        });
    }
    
    // Convert paths
    if (swagger.paths) {
        Object.keys(swagger.paths).forEach(pathKey => {
            openapi.paths[pathKey] = {};
            const pathItem = swagger.paths[pathKey];
            
            Object.keys(pathItem).forEach(method => {
                if (method === 'parameters') {
                    openapi.paths[pathKey].parameters = pathItem[method];
                    return;
                }
                
                const operation = pathItem[method];
                const convertedOp = {
                    ...operation,
                    responses: {}
                };
                
                // Convert parameters
                if (operation.parameters) {
                    convertedOp.parameters = operation.parameters.map(param => {
                        if (param.in === 'body') {
                            // Convert body parameter to requestBody
                            delete convertedOp.parameters;
                            convertedOp.requestBody = {
                                required: param.required || false,
                                content: {
                                    'application/json': {
                                        schema: param.schema
                                    }
                                }
                            };
                            return null;
                        }
                        return param;
                    }).filter(p => p !== null);
                }
                
                // Convert responses
                if (operation.responses) {
                    Object.keys(operation.responses).forEach(statusCode => {
                        const response = operation.responses[statusCode];
                        convertedOp.responses[statusCode] = {
                            description: response.description || ''
                        };
                        
                        if (response.schema) {
                            convertedOp.responses[statusCode].content = {
                                'application/json': {
                                    schema: response.schema
                                }
                            };
                        }
                    });
                }
                
                // Convert produces/consumes to content types
                if (operation.consumes) {
                    delete convertedOp.consumes;
                }
                if (operation.produces) {
                    delete convertedOp.produces;
                }
                
                openapi.paths[pathKey][method] = convertedOp;
            });
        });
    }
    
    // Copy security requirements
    if (swagger.security) {
        openapi.security = swagger.security;
    }
    
    return cleanOpenAPISpec(openapi);
}

// Main execution
try {
    // Read input file
    const inputContent = fs.readFileSync(inputFile, 'utf8');
    const swagger = JSON.parse(inputContent);
    
    // Convert to OpenAPI 3.0 if needed
    const openapi = convertSwaggerToOpenAPI(swagger);
    
    // Convert to YAML
    const yamlContent = yaml.dump(openapi, {
        indent: 2,
        lineWidth: -1,
        noRefs: false,
        sortKeys: false
    });
    
    // Ensure output directory exists
    const outputDir = path.dirname(outputFile);
    if (!fs.existsSync(outputDir)) {
        fs.mkdirSync(outputDir, { recursive: true });
    }
    
    // Write output file
    fs.writeFileSync(outputFile, yamlContent, 'utf8');
    
    console.log(`Successfully converted ${inputFile} to ${outputFile}`);
    process.exit(0);
} catch (error) {
    console.error('Error converting file:', error.message);
    process.exit(1);
}