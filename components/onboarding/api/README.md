# Onboarding API Artifacts

This directory contains the OpenAPI specification and generated Swagger files for the Onboarding service.

- `openapi.yaml` — source specification
- `swagger.yaml` / `swagger.json` — generated artifacts consumed by tooling
- `docs.go` — package comment to expose Swagger via go:generate tooling

Changes to the API should be reflected in `openapi.yaml` and regenerated artifacts.
