# AGENTS.md

Conventions for working in this repo.

## Required arguments: don't nil-check, let it panic

If a function's argument or receiver is required (for example, the
`*ObjectUpdate` passed to `Root.Apply`), do not add `if x == nil`
checks that return an error. Passing nil is a programmer error, not a runtime
condition: let the nil dereference panic, as the standard library does, and say
"x must not be nil" / "x is required" in the doc comment if it isn't obvious.

Do return errors for *invalid values* that callers can plausibly construct at
runtime (e.g. a saved `ObjectUpdate` whose digest algorithm is unsupported), and
for input data problems. Only guard nil where nil is documented as meaningful
(e.g. an optional `ContentSource`, "may be nil").
