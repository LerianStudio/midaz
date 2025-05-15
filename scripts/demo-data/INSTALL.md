# Demo Data Generator Installation Instructions

To fully integrate the Demo Data Generator with the Midaz project, please follow these steps:

## 1. Add to Root Makefile

Add the following line to your project's root `Makefile` after the other includes:

```makefile
include $(MIDAZ_ROOT)/scripts/makefile-demo-data.mk
```

## 2. Add to Help Section

Add an entry to the help section of your root `Makefile` to document the new command:

```makefile
@echo ""
@echo ""
@echo "Data Generation Commands:"
@echo "  make generate-demo-data              - Generate demo data for local development"
@echo "  make generate-demo-data VOLUME=small|medium|large - Specify data volume size"
```

## 3. Verify Installation

Run the command to ensure it's properly installed:

```bash
make generate-demo-data
```

## 4. Usage Options

The Demo Data Generator supports the following options:

- `VOLUME=small|medium|large` - Specify the volume of data to generate
- `AUTH_TOKEN=your_token` - Provide an authentication token for API access
- `BASE_URL=http://your-host` - Specify a custom base URL
- `ONBOARDING_PORT=port` - Specify a custom port for the onboarding service
- `TRANSACTION_PORT=port` - Specify a custom port for the transaction service

Example:

```bash
make generate-demo-data VOLUME=medium AUTH_TOKEN=your_token
```