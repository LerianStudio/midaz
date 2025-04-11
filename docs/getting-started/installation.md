# Installation

**Navigation:** [Home](../../) > [Getting Started](../) > Installation

This guide provides step-by-step instructions for installing and setting up Midaz in your environment.

## Prerequisites

Before installing Midaz, ensure you have the following prerequisites installed:

- **Go 1.24.1 or higher** - [Download and install from golang.org](https://golang.org/doc/install)
- **Docker and Docker Compose** - [Download and install from docker.com](https://docs.docker.com/get-docker/)
- **Git** - [Download and install from git-scm.com](https://git-scm.com/downloads)

## Installation Steps

### 1. Clone the Repository

```bash
git clone https://github.com/LerianStudio/midaz
cd midaz
```

### 2. Set Up Environment Variables

The Midaz system uses environment variables for configuration. Default environment files are provided as templates.

```bash
make set-env
```

This command copies all `.env.example` files to `.env` in each component directory. You may need to modify these files to match your environment.

### 3. Start All Services

To start all Midaz services using Docker Compose:

```bash
make up
```

This command will:
- Start all infrastructure services (PostgreSQL, MongoDB, RabbitMQ, etc.)
- Start the Onboarding service
- Start the Transaction service

### 4. Verify Installation

To verify that all services are running correctly:

```bash
make logs
```

This will show logs from all running services. Ensure there are no error messages.

You can also check the status of all running containers:

```bash
docker ps
```

## Component-specific Installation

### MDZ CLI

To build and install the MDZ CLI locally:

```bash
cd components/mdz
make build
make install-local
```

After installation, you can use the `mdz` command to interact with the Midaz services.

## Configuration

### Default Ports

The following default ports are used by Midaz services:

| Service               | Port  |
|-----------------------|-------|
| Onboarding Service    | 3000  |
| Transaction Service   | 3001  |
| PostgreSQL Primary    | 5701  |
| PostgreSQL Replica    | 5702  |
| MongoDB               | 5703  |
| Redis                 | 5704  |
| RabbitMQ Web UI       | 3003  |
| RabbitMQ AMQP         | 3004  |
| Grafana               | 3100  |

### Environment Configuration

Each component has its own `.env` file with specific configuration options:

- **Infra** - Database connection details, message queue settings
- **Onboarding** - Service-specific configurations
- **Transaction** - Service-specific configurations
- **MDZ CLI** - Client configuration and endpoints

## Development Setup

For a complete development environment setup, run:

```bash
make dev-setup
```

This will:
- Set up git hooks
- Configure development environments for all components
- Install necessary dependencies

## Common Operations

### Starting and Stopping Services

```bash
# Start all services
make up

# Stop all services
make down

# Start all containers without recreating them
make start

# Stop all containers
make stop

# Restart all containers
make restart
```

### Running Tests

```bash
# Run all tests
make test

# Generate test coverage report
make cover
```

### Code Quality

```bash
# Run linters
make lint

# Format code
make format

# Run security checks
make sec
```

## Troubleshooting

### Docker Issues

If you encounter issues with Docker containers:

```bash
# Clean all Docker resources
make clean-docker

# Rebuild and restart all services
make rebuild-up
```

### Missing Environment Files

If you see errors about missing configuration:

```bash
# Re-create environment files
make set-env
```

### Port Conflicts

If you see errors about ports being in use, check for other applications using the ports listed in the "Default Ports" section above. You can modify the port mappings in the `.env` files for each component.

## Next Steps

After successfully installing Midaz, you can:

- Continue to the [Quickstart Guide](./quickstart.md) for basic usage examples
- Explore the [Architecture Overview](./architecture-overview.md) to understand how Midaz works
- Check out the [Tutorials](../tutorials/) for step-by-step guides on specific tasks