# Postman Collection Generation Process

This document outlines the automated process for generating and updating the `MIDAZ.postman_collection.json` and `MIDAZ.postman_environment.json` files located in this directory.

The process is triggered by running the following command from the project root:

```bash
make generate-docs
```

## Detailed Steps

1.  **Initiation (`Makefile`):**
    *   The `make generate-docs` target in the root `Makefile` starts the process.
    *   **Prerequisites:** It first runs `make tidy` (dependency cleanup) and `make check-envs` (environment checks). It also verifies that the `swag` CLI and `node` are installed.

2.  **OpenAPI Specification Generation (`Makefile` + `swag`):
    *   For each relevant service component (`mdz`, `onboarding`, `transaction`):
        *   The command navigates into the component's directory (e.g., `components/onboarding`).
        *   It executes `swag init -g cmd/server/main.go -o docs`.
        *   The `swag` tool parses the Go source code, focusing on files reachable from `cmd/server/main.go`.
        *   It identifies special [Swagger annotations](https://github.com/swaggo/swag#declarative-comments-format) (e.g., `@Summary`, `@Description`, `@Param`, `@Success`, `@Failure`, `@Router`) within Go code comments, typically placed above HTTP handler functions.
        *   Based on these annotations and the code structure, `swag` generates OpenAPI v2 specification files (`swagger.json` and `swagger.yaml`) in the component's `docs/` subdirectory.

3.  **Postman Sync Script Execution (`Makefile`):
    *   After generating OpenAPI specs for all components, the Makefile executes the `./scripts/sync-postman.sh` script.

4.  **Script Setup (`sync-postman.sh`):
    *   **Dependency Checks:** The script ensures `node` and `jq` (JSON processor) are installed, attempting installation if necessary.
    *   **Directory Setup:** Creates `postman/temp` for intermediate files and `postman/backups` for archiving.
    *   **Backup:** Archives existing `MIDAZ.postman_collection.json` and `MIDAZ.postman_environment.json` to the `backups` directory with a timestamp.
    *   **NPM Install:** Runs `npm install` in the `scripts/` directory to ensure Node.js dependencies required by the conversion script (`convert-openapi.js`) are present.

5.  **OpenAPI to Postman Conversion (`sync-postman.sh` + `convert-openapi.js`):
    *   The script iterates through the component OpenAPI specs (`onboarding`, `transaction`).
    *   It uses `node ./scripts/convert-openapi.js` to convert each component's `docs/swagger.json` into a temporary Postman collection (`postman/temp/<component>.postman_collection.json`) and potentially a temporary Postman environment file (`postman/temp/<component>.environment.json`).
    *   The custom `convert-openapi.js` script handles the specific logic of translating OpenAPI definitions into the Postman collection format, potentially including enhanced descriptions, examples, or variable handling.

6.  **Merging Collections & Environments (`sync-postman.sh` + `jq`):
    *   **Collections:** Uses `jq` to merge the temporary Postman collections from the `temp/` directory into the final `postman/MIDAZ.postman_collection.json`.
        *   Combines request items (folders/requests) from all components.
        *   Filters out specific folders (e.g., "E2E Flow") if necessary.
        *   Merges collection variables, ensuring uniqueness.
        *   Sets the final collection name (`MIDAZ`) and a fixed Postman ID.
    *   **Environments:** Similarly, uses `jq` to merge the temporary environment files into `postman/MIDAZ.postman_environment.json`.
        *   Combines environment variables, ensuring unique keys.
        *   Sets the final environment name (`MIDAZ Environment`) and a fixed Postman ID.

7.  **Cleanup (`sync-postman.sh`):
    *   Removes the temporary directory (`postman/temp`).

## Result

The process results in updated `MIDAZ.postman_collection.json` and `MIDAZ.postman_environment.json` files in the `postman/` directory, reflecting the latest API definitions documented via Swagger annotations in the Go code.
