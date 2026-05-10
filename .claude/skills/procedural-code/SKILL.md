---
name: improve-codebase
description: >
  Write, Refactor and improve codebases toward procedural, data-oriented, explicit programming styles
  rooted in systems programming philosophy. Use this skill whenever the user asks to write, refactor code,
  improve code quality, simplify architecture, remove unnecessary abstraction, flatten class
  hierarchies, make code more explicit or obvious, adopt data-oriented design, reduce indirection,
  or improve performance through simpler structures. Also trigger when the user mentions
  "procedural refactor", "remove OOP bloat", "flatten abstractions", "make this obvious",
  "simplify this architecture", "data-oriented", or asks to review code for over-engineering.
---

# Procedural Code

A skill for refactoring code toward clarity, simplicity, and performance by applying procedural
and data-oriented design principles. The core philosophy: code should be obvious, data transformations
should be explicit, and abstractions should be earned — not inherited from a framework or pattern book.

## Core Principles

These aren't arbitrary style preferences. They come from decades of shipping real software
(game engines, compilers, language runtimes) where unnecessary complexity has direct, measurable
costs. The principles reinforce each other — flattening a hierarchy usually also improves data
layout, which also makes the code more obvious.

### 1. Solve the Actual Problem

Before touching any code, identify what the code _actually does_ at the data level. Not what the
class diagram says. Not what the design pattern implies. What bytes go in, what bytes come out,
and what transformations happen in between.

Ask: "If I were writing this from scratch today, knowing exactly what it needs to do, would I
write it this way?" The answer is almost always no — and the gap between the current code and
that hypothetical rewrite is where the improvements live.

### 2. Prefer Plain Data and Functions

Data should be plain structs, records, or simple data structures. Functions should take data in
and produce data out. This is the default. Deviate only when you have a concrete, demonstrable
reason.

**Instead of:**

```
class UserManager:
    def __init__(self, db, cache, logger, event_bus):
        self._db = db
        self._cache = cache
        self._logger = logger
        self._event_bus = event_bus

    def create_user(self, name, email):
        user = User(name, email)
        self._db.save(user)
        self._cache.invalidate("users")
        self._logger.info(f"Created user {user.id}")
        self._event_bus.emit("user_created", user)
        return user
```

**Write:**

```
def create_user(db, name, email):
    user = {"id": generate_id(), "name": name, "email": email}
    db.execute("INSERT INTO users (id, name, email) VALUES (?, ?, ?)",
               (user["id"], user["name"], user["email"]))
    return user
```

The cache invalidation, logging, and event emission were likely speculative. If they're genuinely
needed, add them at the call site where the context makes the reason obvious. Don't pre-wire
every possible side effect into a God object.

### 3. If You Use Classes, Treat Them Like APIs — Not Bureaucracies

When a class does earn its place, treat its boundary like a well-designed API. That means the
public surface should be the minimal set of things external code actually needs — and those
things should be _direct_.

Drop getter/setter ceremony. A private field with a public getter and a public setter is a
public field wearing a disguise. It doesn't protect invariants — it just adds two method calls
that do nothing. If external code needs to read and write a value, make the field public. Full
stop.

**Instead of:**

```
class Vector3:
    def __init__(self, x, y, z):
        self._x = x
        self._y = y
        self._z = z

    def get_x(self): return self._x
    def set_x(self, x): self._x = x
    def get_y(self): return self._y
    def set_y(self, y): self._y = y
    def get_z(self): return self._z
    def set_z(self, z): self._z = z
```

**Write:**

```
class Vector3:
    def __init__(self, x, y, z):
        self.x = x
        self.y = y
        self.z = z
```

Getters and setters earn their existence only when they enforce a real invariant — when setting
a value requires validation, triggers a necessary side effect, or maintains a relationship
between fields that would break if one changed independently. "We might need validation later"
is not a real invariant. Add the getter when you add the validation. Until then, it's just noise
that makes every access site harder to read.

The same principle applies at every boundary: if calling code needs direct access to data, give
it direct access. Don't make callers navigate a maze of accessor methods that exist purely out
of habit. A good class API is flat, obvious, and does not punish the caller for the sin of
wanting to read a field.

### 4. Flatten Abstraction Hierarchies

Every layer of indirection is a cost. It's a place where bugs hide, where performance gets lost,
where a new developer has to stop and trace through another file. Layers should exist only when
they solve a problem you actually have _right now_.

**Symptoms of unnecessary abstraction:**

- A class that wraps another class and delegates most methods unchanged
- An interface with exactly one implementation
- A "factory" that always produces the same type
- An "adapter" or "bridge" between two things you control
- A base class whose only purpose is to be inherited from
- A "service" class that's just a namespace for functions with shared state

**The refactoring move:** inline the abstraction. Pull the implementation up to the call site.
If you see `this.userRepository.findById(id)` and `UserRepository` is a thin wrapper around a
single SQL query, replace it with the query. You can always re-extract later if a real need
emerges — and it probably won't.

### 5. Data-Oriented Design

Think about how data flows through your program. Data that is processed together should live
together. Data that is processed differently should be separated — even if it "logically belongs"
to the same entity.

**Key patterns:**

- **Struct of Arrays over Array of Structs:** If you're iterating over 10,000 entities but only
  touching their positions, don't load their names, textures, and AI states into cache too.
  Separate the arrays.

- **Batch processing over individual operations:** Instead of processing items one at a time
  through a pipeline of virtual method calls, collect items and process them in passes. Each pass
  does one transformation over the whole batch.

- **Linear data flow over scattered mutation:** Data should flow in one direction through a
  transformation pipeline. Avoid patterns where multiple systems reach into shared mutable state
  and modify it at unpredictable times.

### 6. Explicit Over Implicit

The reader should be able to understand what code does by reading it top to bottom. No magic.
No "convention over configuration." No framework that calls your code through reflection.

This means:

- Spell out control flow. An `if` statement is better than a polymorphic dispatch when there are
  three cases and the function is 30 lines.
- Spell out data access. `users[user_id].email` is better than `getCurrentContext().resolve("user").getAttribute("email")`.
- Spell out error handling. Return error values. Check them at the call site. Don't throw
  exceptions across five stack frames into a generic handler.
- Spell out dependencies. Function parameters are better than injected services. A function
  signature that says `fn process(db: *Database, items: []Item) !Result` tells you everything.
  A method on `ProcessingService` that reads from `self._config.processing.batch_size` tells you
  nothing without reading three other files.

### 7. Compression Is Earned, Not Assumed

Don't start with the abstraction. Write the concrete code first — maybe two or three
times. Then, only when you can see the _actual_ repeated pattern, compress it into a shared
function or structure. The abstraction emerges from the code, not from a whiteboard session
before the code exists.

This means: duplicated code is not automatically bad. Two functions that look similar but serve
different purposes should stay separate. They'll diverge as requirements change, and when they
do, you won't have to untangle a premature abstraction.

### 8. Performance Is Not Premature

Thinking about how data moves through memory, how work is batched, and how allocations happen is
not "premature optimisation." It's engineering. The famous "premature optimisation" quote is
about micro-optimisations at the instruction level before you've profiled. It's not a license to
ignore the fact that chasing pointers through a linked list of heap-allocated polymorphic objects
is 100x slower than iterating a flat array.

Design decisions that are easy to get right at the start and expensive to fix later:

- Data layout (arrays vs. pointer graphs)
- Allocation strategy (arena vs. individual alloc/free)
- Batching (one-at-a-time vs. bulk operations)
- Serialisation boundaries (where data crosses process/thread/network lines)

---

## Refactoring Workflow

When asked to improve a codebase, follow this sequence. The goal is to understand before you
change — measure the current state, identify the highest-leverage improvements, and make
targeted changes that each independently leave the code in a better state.

### Step 1: Survey and Understand

Read the code. Identify:

1. **What does this code actually do?** Describe the data transformations in plain language.
   Ignore class names and patterns. What goes in, what comes out?
2. **Where does complexity live?** Count layers of indirection. Trace a single operation from
   entry point to data store and back. How many files do you touch?
3. **Where is data defined vs. where is it used?** Large distances between definition and use
   are a smell.
4. **What's speculative?** Identify abstractions that exist "in case we need to" rather than
   because something concrete requires them today.

### Step 2: Identify Candidates

Prioritise refactoring targets by impact. High-value targets:

- **God objects / manager classes** — classes with 10+ dependencies or methods that coordinate
  everything. Break them into plain functions that operate on data.
- **Deep inheritance hierarchies** — replace with composition of plain data, or often just
  a type tag and a switch statement.
- **Wrapper layers with no logic** — repositories, services, adapters that just forward calls.
  Inline them.
- **Framework ceremony** — decorators, annotations, middleware chains, DI containers that add
  complexity without solving a problem specific to this codebase. Replace with explicit code.
- **Scattered mutation** — multiple systems modifying shared state through different paths.
  Centralise into a single pass with clear ownership.
- **Premature genericisation** — generic/template code used at exactly one call site with
  exactly one type parameter. Monomorphise it.

### Step 3: Refactor in Targeted Passes

Make one kind of improvement at a time. Each pass should leave the code in a working state.

1. **Inline pass:** Remove unnecessary wrappers and delegation layers.
2. **Data pass:** Restructure data for access patterns. Group fields by usage. Replace
   object graphs with flat arrays where appropriate.
3. **Control flow pass:** Replace polymorphic dispatch with explicit conditionals where the
   number of cases is small and stable. Replace visitor patterns with switches.
4. **Dependency pass:** Convert injected dependencies to function parameters. Make data flow
   visible in signatures.
5. **Naming pass:** Rename to reflect what things _are and do_, not what pattern they
   implement. `process_orders(db, orders)` not `OrderProcessingService.execute()`.

### Step 4: Validate

After refactoring:

- Run existing tests. If tests break purely because they tested implementation details
  (mocking specific classes that no longer exist), that's a signal the refactoring was correct
  — the old code had test-induced design damage.
- Verify the same inputs produce the same outputs.
- Check that call stacks are shorter. A good refactoring reduces the average depth of a
  stack trace for common operations.
- Check that file count went down or stayed the same. More files is not more organised; it's
  more indirection.

---

## Language-Specific Guidance

### Python

- Replace class-based services with modules of functions. Python modules are already namespaces.
- Use dataclasses or TypedDicts for plain data, not classes with methods.
- Replace ABC/Protocol hierarchies with simple duck typing or union types.
- Prefer `dict` and `list` over custom collection wrappers unless the wrapper provides real
  invariant enforcement.
- Use explicit error returns (`Result` pattern or tuples) over exceptions for expected failure
  modes. Reserve exceptions for genuinely exceptional situations.

### TypeScript / JavaScript

- Replace class hierarchies with plain objects and discriminated unions.
- Replace DI containers with explicit function parameters or module-level constants.
- Avoid decorator-heavy frameworks (NestJS-style) in favour of explicit routing and middleware.
- Use `interface` for data shapes, not for polymorphic behaviour contracts with one implementor.
- Prefer `Map<K, V>` and flat arrays over nested object graphs.

### Go

- Go already encourages this style. Main pitfall: over-interfacing. An interface with one
  implementation is just indirection. Use the concrete type.
- Avoid "package per type." Group by feature or data flow, not by noun.
- Use struct embedding for composition, not for pseudo-inheritance.
- Prefer table-driven tests over test frameworks with assertion libraries.

### C / Systems Code

- This is the home turf. Structs and functions. Arena allocators. Flat arrays. Explicit lifetime
  management. The principles above are descriptions of how good C code already works.
- Prefer sized arrays and length-tracking over null-terminated strings and sentinel values.
- Use struct-of-arrays for hot paths. Keep cold data in a separate struct.

---

## Anti-Patterns Reference

Recognise these as signals that refactoring is needed:

| Anti-Pattern                      | What It Looks Like                                                            | The Fix                                              |
| --------------------------------- | ----------------------------------------------------------------------------- | ---------------------------------------------------- |
| AbstractSingletonProxyFactoryBean | Class name longer than its implementation                                     | Inline to a function                                 |
| Lasagna code                      | Must read 8 files to trace one operation                                      | Inline layers until the trace fits in 2-3 files      |
| Speculative generality            | `IUserRepository` with only `SqlUserRepository`                               | Use `SqlUserRepository` directly, drop the interface |
| Middleman                         | Class that delegates every method to another class                            | Remove the middleman, call the delegate directly     |
| Framework worship                 | More framework config than business logic                                     | Replace framework magic with explicit code           |
| Getter/Setter theatre             | Private field + public getter + public setter = public field with extra steps | Make the field public or rethink the design          |
| DI container addiction            | Can't instantiate a class without a 200-line config                           | Pass dependencies as function arguments              |
| Event soup                        | 12 event handlers fire in undetermined order to process one request           | Write a single function that does the steps in order |

---

## What This Skill Is NOT

This skill is not about religious adherence to "no classes ever" or "C is the only real language."
It's about pragmatism. The question is always: does this abstraction _earn its keep_? Does this
layer of indirection solve a real problem, or is it cosplaying as architecture?

Sometimes a class is the right answer. Sometimes an interface with multiple implementations is
exactly what you need. The point is to make that choice consciously, based on the actual problem,
rather than defaulting to patterns because they're "best practice."

Good code is code where a new developer can open any file, read it top to bottom, and understand
what it does without consulting a class diagram, a wiki, or a senior engineer's tribal knowledge.
That's the bar. Refactor toward it.
