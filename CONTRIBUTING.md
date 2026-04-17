# Contributing

Thanks for your interest in contributing to `gh-npm-oidc-migrate`.

## Development setup

1. Install Go (version defined in `go.mod`).
2. Clone the repository.
3. Run tests:

   ```bash
   go test ./...
   ```

4. Build locally:

   ```bash
   go build -o gh-npm-oidc-migrate .
   ```

## Pull requests

- Keep changes focused and small.
- Add or update tests when behavior changes.
- Ensure `go test ./...` and `go build ./...` pass before submitting.
- Follow existing code style and project structure.

## Reporting issues

If you find a bug or have a feature request, please open a GitHub issue with
clear reproduction steps and expected behavior.
