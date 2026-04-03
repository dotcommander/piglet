# Pipeline

Runs multi-step shell workflows defined in YAML files with parameters, conditionals, loops, retries, and step output chaining.

## Capabilities

| Capability | Name |
|------------|------|
| tool | `pipeline` |
| command | `pipeline` |
| prompt | `pipeline` |

## CLI Usage

```
pipeline [flags] <file.yaml>
pipeline list [directory]
```

### Flags

| Flag | Description |
|------|-------------|
| `-dry-run` | Preview steps without executing |
| `-param key=value` | Parameter override (repeatable) |
| `-json` | Output as JSON |
| `-q` | Quiet — only errors and final status |

## Pipeline YAML Format

```yaml
name: deploy
description: Build and deploy the app
params:
  env:
    default: staging
    description: Target environment
  version:
    required: true
    description: Release version
steps:
  - name: build
    run: go build -o app ./cmd/
    timeout: 60

  - name: deploy
    run: scp app {param.env}-server:/opt/app-{param.version}
```

## Step Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Step identifier |
| `run` | string | required | Shell command to execute |
| `shell` | string | `sh` | Shell to use |
| `timeout` | int | `30` | Per-step timeout in seconds |
| `retries` | int | `0` | Number of retry attempts |
| `retry_delay` | int | `5` | Seconds between retries |
| `allow_failure` | bool | `false` | Continue pipeline on failure |
| `each` | string[] | | Iterate over a list of values |
| `loop` | map | | Iterate over ranges (cartesian product) |
| `workdir` | string | | Working directory for the step |
| `env` | map | | Extra environment variables |
| `when` | string | | Shell predicate — skip step if exit code != 0 |

## Template Variables

Use `{variable}` syntax in `run`, `workdir`, and `when` fields.

### Parameters

| Variable | Description |
|----------|-------------|
| `{param.<name>}` | Pipeline parameter value |

### Step Output References

| Variable | Description |
|----------|-------------|
| `{prev.stdout}` | Previous step's stdout |
| `{prev.lines}` | Previous step's stdout, trimmed |
| `{prev.status}` | Previous step's status (`ok` or `error`) |
| `{prev.json.<key>}` | Extract a top-level JSON field from previous output |
| `{step.<name>.stdout}` | Named step's stdout |
| `{step.<name>.status}` | Named step's status |

### Loop Variables

| Variable | Description |
|----------|-------------|
| `{item}` | Current value in an `each` iteration |
| `{loop.<key>}` | Current value for a `loop` dimension |

### Built-in Variables

| Variable | Description |
|----------|-------------|
| `{cwd}` | Current working directory |
| `{date}` | Pipeline start date (`YYYY-MM-DD`) |
| `{timestamp}` | Pipeline start time (Unix epoch) |

## Loops and Iteration

### `each` — iterate over a list

```yaml
- name: test-services
  each: [auth, api, worker]
  run: go test ./{item}/...
```

### `loop` — iterate over ranges

Supports explicit lists, numeric ranges, and day ranges:

```yaml
- name: matrix-build
  loop:
    os: [linux, darwin]
    arch: [amd64, arm64]
  run: GOOS={loop.os} GOARCH={loop.arch} go build -o bin/app-{loop.os}-{loop.arch}
```

```yaml
- name: numeric-range
  loop:
    n: "1..5"
  run: echo "iteration {loop.n}"
```

```yaml
- name: daily-report
  loop:
    day: "-7d..-1d"
  run: fetch-report --date {loop.day}
```

### Combined `each` + `loop`

Produces the cartesian product of all values:

```yaml
- name: deploy-all
  each: [us-east, eu-west]
  loop:
    version: [v1.0, v1.1]
  run: deploy --region {item} --version {loop.version}
```

Loop iterations run in parallel (default concurrency: 4, configurable at the pipeline level).

## Conditionals

Use `when` to conditionally skip a step. The value is run as a shell command — the step executes only if the command exits 0:

```yaml
- name: migrate
  when: "test {param.env} = production"
  run: migrate-db --prod
```

Template variables are expanded in `when` before evaluation.

## Retries

```yaml
- name: flaky-deploy
  run: deploy.sh
  retries: 3
  retry_delay: 10
```

## Allowing Failures

Steps with `allow_failure: true` don't halt the pipeline. The final status will be `partial` instead of `error`:

```yaml
- name: optional-lint
  run: golangci-lint run
  allow_failure: true
```

## Output Chaining

Steps can reference output from previous steps:

```yaml
steps:
  - name: get-version
    run: git describe --tags

  - name: build
    run: go build -ldflags "-X main.version={prev.stdout}" -o app

  - name: tag-check
    run: curl -s https://api.example.com/releases/latest

  - name: compare
    run: echo "Latest release is {step.tag-check.stdout}"
```

## Listing Pipelines

```
pipeline list ./pipelines
```

Lists all `.yaml`/`.yml` files in a directory with their names and descriptions.

## Examples

```bash
# Run a pipeline
pipeline deploy.yaml

# Dry run
pipeline -dry-run deploy.yaml

# Override parameters
pipeline -param env=production -param version=2.0.0 deploy.yaml

# JSON output for scripting
pipeline -json build.yaml

# Quiet mode
pipeline -q test.yaml
```
