# Contributing to Documentation

**Navigation:** [Home](./) > Contributing to Documentation

This guide explains how to contribute to the Midaz documentation, including our documentation structure, style guidelines, and review process. High-quality documentation is critical to the success of Midaz, and we welcome contributions from everyone.

## Table of Contents

- [Documentation Philosophy](#documentation-philosophy)
- [Documentation Structure](#documentation-structure)
- [Getting Started](#getting-started)
- [Documentation Guidelines](#documentation-guidelines)
- [Markdown Style Guide](#markdown-style-guide)
- [Writing Best Practices](#writing-best-practices)
- [Adding Images and Diagrams](#adding-images-and-diagrams)
- [Creating Tutorials](#creating-tutorials)
- [Submitting Changes](#submitting-changes)
- [Documentation Review Process](#documentation-review-process)

## Documentation Philosophy

Midaz documentation follows these core principles:

1. **User-Centered**: Documentation is written with the user's needs in mind
2. **Comprehensive**: Covers all aspects of the platform, from high-level concepts to detailed implementation
3. **Well-Structured**: Organized in a logical, intuitive manner with clear navigation
4. **Current**: Regularly updated to reflect the latest features and changes
5. **Accessible**: Written in clear, concise language accessible to users with varying levels of expertise

## Documentation Structure

Our documentation is organized into the following key sections:

1. **Getting Started**: Installation, quickstart guides, and basic setup
2. **Architecture**: System overview, architectural patterns, and component interactions
3. **Components**: Detailed documentation for each system component
4. **Developer Guide**: Guidelines for developers contributing to the project
5. **Tutorials**: Step-by-step guides for common tasks
6. **Reference**: API documentation, glossary, and other reference materials
7. **Troubleshooting**: Common issues and their solutions

## Getting Started

### Prerequisites

To contribute to the documentation, you'll need:

- Git installed on your local machine
- A GitHub account
- Basic knowledge of Markdown
- A text editor (VS Code, Sublime Text, etc.)

### Setting Up Your Environment

1. **Fork and Clone the Repository**

   ```bash
   git clone https://github.com/YOUR-USERNAME/midaz.git
   cd midaz
   git remote add upstream https://github.com/LerianStudio/midaz.git
   ```

2. **Navigate to the Documentation Directory**

   ```bash
   cd docs
   ```

3. **Create a Branch for Your Changes**

   ```bash
   git checkout -b docs/your-documentation-change
   ```

## Documentation Guidelines

### General Guidelines

1. **Know Your Audience**:
   - Consider who will be reading the documentation (developers, operators, end-users)
   - Adjust the technical level accordingly

2. **Be Clear and Concise**:
   - Use simple, direct language
   - One idea per paragraph
   - Avoid jargon and acronyms without explanation

3. **Document Structure**:
   - Start with a clear title and introduction
   - Use consistent headings and subheadings
   - Include a table of contents for longer documents
   - End with a conclusion or next steps when appropriate

4. **Keep It Current**:
   - Documentation should reflect the current state of the software
   - Update documentation when features change
   - Remove outdated information

5. **Cross-Reference**:
   - Link to related documentation
   - Avoid duplication by referencing existing content
   - Ensure links are valid and point to the correct location

### Documentation Types

#### Conceptual Documentation

- Explains concepts, architectures, and the "why" behind features
- Includes diagrams and high-level explanations
- Helps users understand the bigger picture

#### Procedural Documentation

- Provides step-by-step instructions
- Uses clear, numbered steps
- Includes expected outcomes and verification steps

#### Reference Documentation

- Provides detailed technical information
- Includes complete API references, configuration options, etc.
- Focuses on accuracy and completeness

#### Troubleshooting Documentation

- Addresses common problems and their solutions
- Includes symptoms, causes, and resolutions
- Uses a consistent problem-solution format

## Markdown Style Guide

Midaz documentation uses GitHub-flavored Markdown. Here are our style conventions:

### File Naming and Organization

- Use lowercase for filenames
- Use hyphens instead of spaces (`contributing-to-docs.md`, not `contributing to docs.md`)
- Place files in the appropriate directory based on content type

### Headers

- Use ATX-style headers (`#` for H1, `##` for H2, etc.)
- Include one space after the `#` symbol
- Use title case for headers
- Separate headers from surrounding text with a blank line

```markdown
## Header Example

Content goes here...
```

### Lists

- Use `-` for unordered lists
- Use `1.`, `2.`, etc. for ordered lists
- Indent nested lists with 2 spaces
- Include a blank line before and after lists

```markdown
- Item 1
- Item 2
  - Nested item 1
  - Nested item 2
- Item 3

1. First step
2. Second step
   1. Sub-step 1
   2. Sub-step 2
3. Third step
```

### Code Blocks

- Use triple backticks (```) for code blocks
- Specify the language for syntax highlighting
- Use inline code with single backticks for small code snippets

    ```go
    // Example Go code
    func main() {
        fmt.Println("Hello, Midaz!")
    }
    ```

### Links

- Use descriptive link text
- Use relative links for internal documentation
- Use absolute links for external resources

```markdown
[Contributing Guide](../developer-guide/contributing.md)
[Go Documentation](https://golang.org/doc/)
```

### Tables

- Use standard Markdown tables
- Include a header row
- Align columns for readability in the Markdown source

```markdown
| Name | Type | Description |
|------|------|-------------|
| id   | string | Unique identifier |
| name | string | User's display name |
```

### Images

- Place images in the `/docs/assets` directory
- Use descriptive alt text
- Include captions when necessary
- Optimize image size for web viewing

```markdown
![Alt text](../assets/example-diagram.png)
*Caption: System architecture diagram*
```

### Admonitions

Use the following format for notes, warnings, tips, and important information:

```markdown
> **Note**: This is a note.

> **Warning**: This is a warning.

> **Tip**: This is a tip.

> **Important**: This is important information.
```

## Writing Best Practices

### Voice and Tone

- Use an active voice rather than passive
- Write in a professional but conversational tone
- Address the reader directly ("you")
- Be positive and supportive
- Maintain a consistent tone across documentation

### Language

- Use American English spelling
- Avoid contractions in formal documentation
- Use present tense when possible
- Be consistent with terminology
- Define technical terms when first used or link to the glossary

### Examples

- Include practical, realistic examples
- Provide complete code samples that can be copied and used directly
- Explain the purpose and expected outcome of examples
- Use consistent formatting for input and output

## Adding Images and Diagrams

### Image Guidelines

1. **Format**: Use PNG for screenshots and diagrams, JPEG for photos
2. **Size**: Optimize images for web (typically under 500KB)
3. **Resolution**: Use appropriate resolution for the content (typically 1200px max width)
4. **Naming**: Use descriptive filenames (`transaction-flow-diagram.png`, not `image1.png`)

### Diagram Tools

- [draw.io](https://draw.io/) (free, web-based and desktop app)
- [Lucidchart](https://www.lucidchart.com/) (freemium)
- [Mermaid](https://mermaid-js.github.io/mermaid/#/) (text-based diagramming integrated with Markdown)

### Diagram Standards

- Use consistent colors and shapes across diagrams
- Include a legend for complex diagrams
- Ensure text is readable at the final display size
- Use arrows to indicate flow direction

## Creating Tutorials

Effective tutorials have these components:

1. **Introduction**: What the tutorial covers and what the reader will accomplish
2. **Prerequisites**: What the reader needs before starting
3. **Steps**: Clear, numbered instructions with code samples and explanations
4. **Verification**: How to confirm each step was completed successfully
5. **Troubleshooting**: Common issues and their solutions
6. **Conclusion**: Summary of what was learned and next steps

Use screenshots or diagrams to illustrate complex steps.

## Submitting Changes

1. **Make Your Changes**:
   - Edit existing files or create new ones following our guidelines
   - Commit your changes with a clear message (e.g., `docs: add transaction tutorial`)

2. **Test Your Changes**:
   - Preview the rendered Markdown to ensure correct formatting
   - Check that all links work correctly
   - Verify that code samples are correct and properly formatted

3. **Push Your Changes**:
   ```bash
   git push origin docs/your-documentation-change
   ```

4. **Create a Pull Request**:
   - Submit a pull request to the `main` branch
   - Include a clear title and description
   - Reference any related issues
   - Request review from documentation maintainers

## Documentation Review Process

1. **Initial Review**: A documentation maintainer will review your submission for formatting, organization, and basic completeness.

2. **Technical Review**: Technical experts will verify the accuracy of technical content.

3. **Editorial Review**: An editor will review for clarity, consistency, and adherence to style guidelines.

4. **Feedback**: You'll receive feedback and may be asked to make revisions.

5. **Approval and Merge**: Once approved, your contribution will be merged into the main documentation.

### Review Criteria

Documentation is evaluated based on:
- Accuracy and technical correctness
- Clarity and readability
- Completeness and coverage
- Organization and structure
- Consistency with style guidelines
- Quality of examples and illustrations

## Documentation Maintenance

### Versioning

- Documentation is versioned alongside the software
- Changes to documentation should align with the appropriate version

### Deprecation

- Mark deprecated features clearly
- Include migration guidance when documenting deprecations
- Remove documentation for removed features when appropriate

---

Thank you for contributing to the Midaz documentation! Your efforts help make our platform more accessible and easier to use for everyone.

If you have questions or need assistance, please contact the documentation team or open an issue on GitHub.