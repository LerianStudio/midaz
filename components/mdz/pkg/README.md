# MDZ CLI - Packages

## Overview

The `pkg` directory contains reusable packages for the MDZ CLI application. These packages provide command implementations, utilities, and infrastructure for the CLI.

## Package Structure

```
pkg/
├── cmd/                 # Cobra command implementations
│   ├── root/           # Root command
│   ├── login/          # Login command
│   ├── configure/      # Configure command
│   ├── version/        # Version command
│   ├── organization/   # Organization commands
│   ├── ledger/         # Ledger commands
│   ├── account/        # Account commands
│   ├── asset/          # Asset commands
│   ├── portfolio/      # Portfolio commands
│   ├── segment/        # Segment commands
│   └── utils/          # Command utilities
├── environment/        # Environment configuration
├── factory/            # Dependency injection
├── iostreams/          # I/O stream abstractions
├── output/             # Output formatting
├── setting/            # Settings management
├── tui/                # Terminal UI components
├── ptr/                # Pointer utilities
└── mockutil/           # Testing utilities
```

## Key Packages

### cmd/

Command implementations using Cobra framework:

- **root/**: Root command with global flags and subcommands
- **login/**: Authentication commands (browser and terminal)
- **configure/**: API endpoint and credential configuration
- **version/**: Display CLI version
- **organization/**: Organization CRUD commands
- **ledger/**: Ledger CRUD commands
- **account/**: Account CRUD commands
- **asset/**: Asset CRUD commands
- **portfolio/**: Portfolio CRUD commands
- **segment/**: Segment CRUD commands
- **utils/**: Shared command utilities

### environment/

Environment configuration management:

- Loads API endpoints from config
- Manages authentication credentials
- Handles version information
- Supports build-time configuration

### factory/

Dependency injection container:

- Creates HTTP clients
- Manages I/O streams
- Provides environment configuration
- Handles global flags

### iostreams/

I/O stream abstractions:

- Wraps stdin, stdout, stderr
- Enables testability
- Supports output redirection

### output/

Output formatting utilities:

- Colored output for terminal
- Success message formatting
- Error message formatting
- No-color mode for CI/CD

### tui/

Terminal UI components:

- Interactive input prompts
- Password input (hidden)
- Select menus
- User-friendly interactions

### setting/

Settings persistence:

- Save/load configuration
- Manage credentials
- Store API endpoints

## Command Structure

### Cobra Command Pattern

All commands follow a consistent structure:

```go
// Command definition
func NewCmdCreate(f *factory.Factory) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "create",
        Short: "Create a new entity",
        RunE:  runCreate(f),
    }

    // Add flags
    cmd.Flags().StringVar(&name, "name", "", "Entity name")

    return cmd
}

// Command execution
func runCreate(f *factory.Factory) func(*cobra.Command, []string) error {
    return func(cmd *cobra.Command, args []string) error {
        // 1. Get repository
        repo := rest.NewOrganization(f)

        // 2. Call API
        entity, err := repo.Create(input)
        if err != nil {
            return err
        }

        // 3. Format output
        output.FormatAndPrint(f, entity.ID, "organization", output.Created)
        return nil
    }
}
```

### Command Hierarchy

```
mdz
├── login              # Authentication
├── configure          # Configuration
├── version            # Version info
├── organization       # Organization commands
│   ├── create
│   ├── list
│   ├── describe
│   ├── update
│   └── delete
├── ledger             # Ledger commands
│   ├── create
│   ├── list
│   ├── describe
│   ├── update
│   └── delete
├── account            # Account commands
│   ├── create
│   ├── list
│   ├── describe
│   ├── update
│   └── delete
├── asset              # Asset commands
│   ├── create
│   ├── list
│   ├── describe
│   ├── update
│   └── delete
├── portfolio          # Portfolio commands
│   ├── create
│   ├── list
│   ├── describe
│   ├── update
│   └── delete
└── segment            # Segment commands
    ├── create
    ├── list
    ├── describe
    ├── update
    └── delete
```

## Testing

### Golden File Testing

Commands use golden file testing for output validation:

```
cmd/organization/testdata/
├── output_list.golden      # Expected list output
└── output_describe.golden  # Expected describe output
```

### Mock Utilities

The `mockutil` package provides testing utilities:

- Mock HTTP clients
- Mock I/O streams
- Mock factories

## Usage Examples

### Using Factory

```go
env := environment.New()
f := factory.NewFactory(env)
f.Token = "your-auth-token"

// Use factory in commands
cmd := root.NewCmdRoot(f)
cmd.Execute()
```

### Custom Output

```go
// Print success message
output.FormatAndPrint(f, "123e4567-...", "organization", output.Created)

// Print error
output.Errorf(f.IOStreams.Err, err)

// Print custom message
output.Printf(f.IOStreams.Out, "Custom message")
```

## Related Documentation

- [MDZ CLI README](../README.md) - CLI overview
- [Internal README](../internal/README.md) - Internal architecture
- [Components README](../../README.md) - All Midaz components
