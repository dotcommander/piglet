# Planning SOP

Follow these phases in order. Do not skip phases. Produce explicit output for each.

## Phase 1: Requirements Gathering

Before any research or design:

1. Restate the request in your own words (one paragraph).
2. List explicit requirements — things the user stated.
3. List implicit requirements — things the user clearly needs but did not state.
4. List constraints — performance, compatibility, size, time, reversibility.
5. List open questions — ambiguities that could change the design. Ask only what you cannot infer.
6. Confirm scope boundary: what is explicitly OUT of scope.

**Gate**: Do not proceed until requirements are clear. If there are blocking open questions, surface them now.

## Phase 2: Research

Investigate before designing:

1. Identify what already exists in the codebase that is relevant.
2. Identify what packages, libraries, or tools are applicable.
3. Identify prior art — similar problems solved elsewhere in the repo.
4. Identify risks — what could go wrong, what is unknown.
5. Identify dependencies — what must exist before this can be implemented.

**Output**: A numbered list of findings. Cite file:line for code references.

## Phase 3: Design

Produce a concrete design before writing code:

1. Name the primary abstraction (type, function, interface, or module).
2. Define the public API surface — signatures, parameters, return types.
3. Define the data flow — input → transform → output.
4. Identify side effects — files written, state mutated, network calls made.
5. Identify failure modes and how each is handled.
6. Choose between alternatives if more than one design is viable — explain the tradeoff.

**Output**: A design summary (not code). Pseudocode is acceptable; real code is premature.

## Phase 4: Step-by-Step Plan

Break the design into atomic, verifiable steps:

1. Each step must be independently completable and testable.
2. Order steps so earlier steps unblock later ones.
3. Mark which steps touch existing files (modify) vs create new files (create).
4. Estimate complexity: trivial / moderate / complex.
5. Identify the first step that produces visible, runnable output.

**Format**: Numbered list. Each item: `[create|modify] <file or component> — <one-line description> (<complexity>)`.

## Phase 5: Risk Assessment

Before implementing:

1. What is the highest-risk step? Why?
2. What would cause this plan to fail entirely?
3. What assumptions are you making that could be wrong?
4. What is the rollback strategy if the implementation fails?
5. Are there any irreversible actions? Flag them explicitly.

## Phase 6: Verification Plan

Define done:

1. How will you verify correctness? (tests, build, manual check)
2. What specific test cases cover the happy path?
3. What edge cases must be tested?
4. What does the build/lint/vet output look like when successful?
5. Is there a smoke test that proves the feature works end-to-end?

**Gate**: Do not consider implementation complete until every verification step passes.

---

Now apply this SOP to the following topic.
