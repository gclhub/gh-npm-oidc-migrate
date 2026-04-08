# gh-npm-oidc-migrate

A GitHub CLI extension that analyzes and migrates npm publishing GitHub Actions
workflows from using fixed NPM tokens (`NODE_AUTH_TOKEN` / `NPM_TOKEN`) to
OIDC-based trusted publishing with ephemeral credentials.

## Installation

```bash
gh extension install gclhub/gh-npm-oidc-migrate
```

## Usage

### Analyze workflows

Scan your repository's GitHub Actions workflows for npm publish steps that use
static tokens:

```bash
# Analyze workflows in the current directory
gh npm-oidc-migrate analyze

# Analyze workflows in a specific directory
gh npm-oidc-migrate analyze /path/to/repo
```

**Example output:**

```
Scanned 2 workflow file(s) in .github/workflows

⚠️  Found 1 finding(s):

  1. Step "npm publish --access public" in job "publish" uses npm publish with
     static token NODE_AUTH_TOKEN referencing ${{ secrets.NPM_TOKEN }}
     File: .github/workflows/publish.yml
     Job:  publish (step 2)

Run 'gh npm-oidc-migrate migrate' to convert these workflows to OIDC.
```

### Migrate workflows

Convert workflows from static tokens to OIDC-based trusted publishing:

```bash
# Preview changes without modifying files
gh npm-oidc-migrate migrate --dry-run

# Apply changes
gh npm-oidc-migrate migrate

# Migrate a specific directory
gh npm-oidc-migrate migrate /path/to/repo
```

The migration performs the following changes:

- **Adds `permissions: id-token: write`** to jobs that run `npm publish`,
  enabling the OIDC token exchange with npm.
- **Removes `NODE_AUTH_TOKEN` and `NPM_TOKEN` environment variables** that
  reference repository secrets.
- **Preserves all other workflow configuration** including non-publish jobs,
  other environment variables, and workflow triggers.

## What it detects

The analyzer checks for npm publish steps across all workflow files
(`.github/workflows/*.yml` and `.github/workflows/*.yaml`) and identifies:

| Pattern | Example |
|---------|---------|
| Step-level token env vars | `env: NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}` |
| Job-level token env vars | `env: NPM_TOKEN: ${{ secrets.NPM_PUBLISH_TOKEN }}` |
| Workflow-level token env vars | Top-level `env:` with token references |

## Additional setup steps

After running the migration, you must complete these additional configuration
steps for OIDC-based publishing to work:

### 1. Configure Trusted Publishing on npmjs.com

1. Go to your package settings at `https://www.npmjs.com/package/<your-package>/access`
2. Under **Trusted Publisher**, select **GitHub Actions**
3. Enter:
   - **Owner/Organization**: Your GitHub username or organization
   - **Repository**: The repository name
   - **Workflow filename**: The workflow file name (e.g., `publish.yml`)
   - **Environment** (optional): The GitHub Actions environment name
4. Save the configuration

> **Note**: The package must already exist on npm. The first version must be
> published manually or with a token before trusted publishing can be configured.

### 2. Ensure npm CLI compatibility

OIDC-based trusted publishing requires:
- **npm v11.5.1** or newer
- **Node.js 22** or newer

If your workflow uses an older Node.js version, add an npm upgrade step:

```yaml
- name: Upgrade npm
  run: npm install -g npm@latest
```

### 3. Remove the old NPM token secret

1. Go to your repository **Settings → Secrets and variables → Actions**
2. Delete the `NPM_TOKEN` (or `NODE_AUTH_TOKEN`) secret
3. This prevents the old token from being used and reduces secret sprawl

### 4. Verify the migration

1. Create a test release or manually trigger the publish workflow
2. Check the GitHub Actions run logs for successful OIDC authentication
3. Verify the npm package page shows a **provenance badge**

## Example

**Before migration:**

```yaml
name: Publish
on:
  release:
    types: [created]
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          registry-url: 'https://registry.npmjs.org'
      - run: npm publish --access public
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

**After migration:**

```yaml
name: Publish
on:
  release:
    types: [created]
jobs:
  publish:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          registry-url: 'https://registry.npmjs.org'
      - run: npm publish --access public
```

## Building from source

```bash
go build -o gh-npm-oidc-migrate .
```

## Running tests

```bash
go test ./...
```

## Learn more

- [npm Trusted Publishing documentation](https://docs.npmjs.com/trusted-publishers)
- [GitHub Actions OIDC documentation](https://docs.github.com/en/actions/security-for-github-actions/security-hardening-your-deployments/about-security-hardening-with-openid-connect)
- [npm trusted publishing announcement](https://github.blog/changelog/2025-07-31-npm-trusted-publishing-with-oidc-is-generally-available/)