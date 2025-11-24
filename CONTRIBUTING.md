# Contributing to FastGo Backend

Thank you for your interest in contributing to FastGo Backend! This document provides guidelines and instructions for contributing.

## Code of Conduct

- Be respectful and inclusive
- Welcome newcomers and help them learn
- Focus on constructive feedback
- Respect different viewpoints and experiences

## How to Contribute

### Reporting Bugs

If you find a bug, please create an issue with:

1. **Clear title** - Brief description of the issue
2. **Description** - Detailed explanation of the problem
3. **Steps to reproduce** - How to reproduce the issue
4. **Expected behavior** - What should happen
5. **Actual behavior** - What actually happens
6. **Environment** - Go version, OS, etc.
7. **Logs** - Relevant error messages or logs

### Suggesting Features

Feature suggestions are welcome! Please create an issue with:

1. **Clear title** - Brief description of the feature
2. **Use case** - Why this feature would be useful
3. **Proposed solution** - How you envision it working
4. **Alternatives** - Other solutions you've considered

### Pull Requests

1. **Fork the repository**
2. **Create a branch** from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```
3. **Make your changes** following the coding standards
4. **Write tests** for new functionality
5. **Update documentation** if needed
6. **Commit your changes**:
   ```bash
   git commit -m "Add: description of your changes"
   ```
7. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```
8. **Create a Pull Request** with a clear description

## Coding Standards

### Go Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` to format code
- Run `go vet` and `golangci-lint` before committing
- Keep functions small and focused
- Write clear, self-documenting code

### Architecture

- Follow Clean Architecture principles
- Keep layers separated (Domain, UseCase, Repository, API)
- Use interfaces for dependencies
- Write tests for each layer

### Naming Conventions

- **Packages**: lowercase, single word
- **Public functions/types**: PascalCase
- **Private functions/types**: camelCase
- **Constants**: PascalCase or UPPER_CASE
- **Files**: snake_case (e.g., `user_repo.go`)

### Code Organization

```
domain/          # Domain entities and business rules
usecase/         # Business logic
repository/      # Data access interfaces
api/             # HTTP handlers
internal/        # Internal implementation
```

## Testing

### Writing Tests

- Write tests for all new functionality
- Use table-driven tests where appropriate
- Mock external dependencies
- Test both success and error cases

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
go test -cover ./...

# Run tests for specific package
go test ./usecase/task
```

### Test Coverage

- Aim for at least 80% coverage for new code
- Focus on testing business logic
- Integration tests for critical paths

## Documentation

### Code Comments

- Comment exported functions and types
- Explain "why", not "what"
- Use clear, concise language
- Follow Go comment conventions

### Documentation Updates

- Update README.md if adding new features
- Add examples to docs/examples/ if applicable
- Update API documentation if changing endpoints

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <subject>

<body>

<footer>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

### Examples

```
feat: add user authentication endpoint

Add POST /api/v1/auth/login endpoint with JWT token generation.

Closes #123
```

```
fix: correct task status validation

Task status was not being validated correctly, allowing invalid values.

Fixes #456
```

## Pull Request Process

1. **Ensure tests pass**:
   ```bash
   make test
   ```

2. **Check code style**:
   ```bash
   make lint
   ```

3. **Update documentation** if needed

4. **Create PR** with:
   - Clear title and description
   - Reference to related issues
   - Screenshots (if UI changes)
   - Test results

5. **Respond to feedback** promptly

6. **Keep PR focused** - One feature or fix per PR

## Review Process

- Maintainers will review your PR
- Address feedback promptly
- Be open to suggestions
- Keep discussions constructive

## Questions?

If you have questions:

1. Check existing [documentation](./docs/README.md)
2. Search existing issues
3. Create a new issue with the `question` label

## Thank You!

Your contributions make this project better. Thank you for taking the time to contribute! 🎉

