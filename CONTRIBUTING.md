# 🤝 Contributing to MariaDB Backup to S3

Thank you for your interest in contributing to the MariaDB Backup to S3 project! We welcome contributions from the community and are grateful for your help in making this tool better.

This document provides guidelines for contributing to the project, including development setup, coding standards, testing procedures, and the contribution workflow.

## Table of Contents
- [🤝 Contributing to MariaDB Backup to S3](#-contributing-to-mariadb-backup-to-s3)
  - [Table of Contents](#table-of-contents)
  - [Development Environment Setup](#development-environment-setup)
    - [Prerequisites](#prerequisites)
    - [Installation](#installation)
    - [Project Dependencies](#project-dependencies)
  - [Building Instructions](#building-instructions)
    - [Local Development Build](#local-development-build)
    - [Cross-Platform Builds](#cross-platform-builds)
  - [Testing Guidelines](#testing-guidelines)
    - [Running Tests](#running-tests)
    - [Test Structure](#test-structure)
    - [CI/CD Testing](#cicd-testing)
  - [Pull Request Workflow](#pull-request-workflow)
    - [Before Creating a PR](#before-creating-a-pr)
    - [Creating the Pull Request](#creating-the-pull-request)
    - [PR Review Process](#pr-review-process)
    - [PR Artifacts](#pr-artifacts)
  - [Code Style Conventions](#code-style-conventions)
    - [Go Standards](#go-standards)
    - [Code Quality Tools](#code-quality-tools)
    - [Project-Specific Conventions](#project-specific-conventions)
    - [Commit Guidelines](#commit-guidelines)
  - [Release Process](#release-process)
    - [Versioning](#versioning)
    - [Release Workflow](#release-workflow)
    - [Pre-Release Process](#pre-release-process)
    - [Post-Release Tasks](#post-release-tasks)
  - [Getting Help](#getting-help)
  - [Recognition](#recognition)

## Development Environment Setup

### Prerequisites

- **Go 1.22.4 or later** - Required for building and running the project
- **MariaDB server** - For testing backup functionality locally
- **Git** - For version control and contribution workflow

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/capcom6/mariadb-backup-s3.git
   cd mariadb-backup-s3
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Set up MariaDB for testing** (optional but recommended)
   ```bash
   # Using Docker for local testing
   docker run --name mariadb-test \
     -e MYSQL_ROOT_PASSWORD=test_password \
     -e MYSQL_DATABASE=test_db \
     -p 3306:3306 \
     -v ${PWD}/mariadb-data:/var/lib/mysql \
     -d mariadb:latest
   ```

4. **Verify setup**
   ```bash
   go version  # Should show 1.22.4 or later
   go build -o mariadb-backup-s3
   ./mariadb-backup-s3 --help
   ```

### Project Dependencies

The project uses the following key dependencies:

- **AWS SDK for Go v2** - For S3-compatible storage integration
- **godotenv** - For loading environment variables from `.env` files
- **envconfig** - For structured configuration management

All dependencies are managed through Go modules. See [`go.mod`](go.mod) for the complete dependency list.

## Building Instructions

### Local Development Build

For local development and testing:

```bash
go build -o mariadb-backup-s3
```

This creates a binary optimized for your current platform.

### Cross-Platform Builds

For building releases across multiple platforms, the project uses [GoReleaser](https://goreleaser.com/):

```bash
# Install GoReleaser (one-time)
go install github.com/goreleaser/goreleaser@latest

# Create a snapshot build (for testing)
goreleaser release --snapshot --clean

# Full release build (requires proper configuration)
goreleaser release --clean
```

## Testing Guidelines

### Running Tests

The project uses Go's built-in testing framework. Run tests using:

```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run tests with coverage
go test -race -coverprofile=coverage.out -covermode=atomic ./...

# View coverage report
go tool cover -html=coverage.out
```

### Test Structure

- Tests are located alongside source code in `*_test.go` files
- Use table-driven tests for multiple test cases
- Follow Go naming conventions: `TestFunctionName`
- Include benchmarks where performance is critical

### CI/CD Testing

All tests run automatically in GitHub Actions:

- **Unit tests** - Run on every push and PR
- **Linting** - Uses golangci-lint for code quality checks
- **Coverage** - Generated locally and/or uploaded by CI, as configured in the repository workflows

## Pull Request Workflow

### Before Creating a PR

1. **Fork the repository** and create a feature branch:
   ```bash
   git checkout -b feature/your-feature-name
   # or for bug fixes
   git checkout -b fix/issue-description
   ```

2. **Ensure tests pass locally**:
   ```bash
   go test -race -cover ./...
   ```

3. **Run linting locally**:
   ```bash
   golangci-lint run
   ```

4. **Update documentation** if needed (README, examples, etc.)

### Creating the Pull Request

1. **Push your branch** to your fork:
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Create a Pull Request** on GitHub with:
   - Clear, descriptive title
   - Detailed description of changes
   - Reference any related issues
   - Screenshots or examples if UI-related

### PR Review Process

1. **Automated Checks**: GitHub Actions will run:
   - Linting with golangci-lint
   - Unit and integration tests
   - GoReleaser snapshot build
   - Artifact generation for testing

2. **Code Review**: At least one maintainer will review your PR
   - Address review comments
   - Make requested changes
   - Ensure all CI checks pass

3. **Testing**: PR artifacts are automatically generated and linked in the PR for testing

4. **Approval and Merge**: Once approved, a maintainer will merge your PR

### PR Artifacts

For each PR, GitHub Actions automatically:
- Builds binaries for multiple platforms
- Creates Docker images tagged as `pr-{number}`
- Uploads artifacts to S3 for testing
- Posts a comment with download links

## Code Style Conventions

### Go Standards

This project follows standard Go conventions and best practices:

- **Formatting**: Use `gofmt` or `goimports` for consistent formatting
- **Naming**: Follow Go naming conventions: exported identifiers use PascalCase (UpperCamelCase); unexported identifiers start with a lowercase letter (e.g., camelCase).
- **Documentation**: Document all exported functions, types, and constants with comments
- **Error Handling**: Use proper error handling patterns, not panic for expected errors

### Code Quality Tools

The project uses golangci-lint for code quality assurance.

### Project-Specific Conventions

1. **Configuration**: Use the `envconfig` package for configuration management
2. **Logging**: Use structured logging with context where appropriate
3. **Error Messages**: Provide clear, actionable error messages
4. **Interfaces**: Define interfaces for pluggable components (like storage backends)

### Commit Guidelines

- Use clear, descriptive commit messages
- Start with a verb (Add, Fix, Update, Remove, etc.)
- Keep commits focused on a single change
- Reference issues when relevant: `Fix issue #123: resolve backup timeout`

## Release Process

### Versioning

The project follows semantic versioning (SemVer):

- **Major** (`x.0.0`): Breaking changes
- **Minor** (`x.y.0`): New features, backwards compatible
- **Patch** (`x.y.z`): Bug fixes, backwards compatible

### Release Workflow

1. **Version Tags**: Releases are triggered by version tags:
   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```

2. **Automated Release**: GitHub Actions automatically:
   - Runs full test suite
   - Builds cross-platform binaries with GoReleaser
   - Creates GitHub release with release notes
   - Builds and pushes Docker images to GitHub Container Registry

3. **Release Artifacts**:
   - Binaries for multiple platforms (Linux, macOS, Windows)
   - Docker images tagged with version
   - SHA256 checksums for verification

### Pre-Release Process

Before creating a release:

1. **Update version numbers** in relevant files
2. **Update CHANGELOG.md** with release notes
3. **Run full test suite** locally
4. **Test release artifacts** in staging environment
5. **Update documentation** if needed

### Post-Release Tasks

After release:
- Monitor for any immediate issues
- Update documentation links if needed
- Announce release in relevant channels
- Plan next development cycle

---

## Getting Help

- **Issues**: Use GitHub issues for bugs, feature requests, or questions
- **Discussions**: Use GitHub discussions for general questions or ideas
- **Documentation**: Check the [README.md](README.md) for usage instructions

## Recognition

Contributors will be recognized in:
- Release notes for significant contributions
- GitHub's contributor list
- Project documentation where appropriate

Thank you for contributing to MariaDB Backup to S3! 🚀
