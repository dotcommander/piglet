Use the pipeline tool to run multi-step workflows. Pipelines are YAML files in ~/.config/piglet/pipelines/.

Features:
- Sequential steps with output passing ({prev.stdout}, {prev.json.<key>})
- Parameter defaults and overrides ({param.<name>})
- Loop constructs: each (list iteration), loop (ranges), cartesian product
- Retry with backoff, allow_failure, when predicates, per-step timeouts
- Dry-run mode to preview without executing

Use pipeline_list to discover available pipelines. Use /pipe-new to create templates.
