# Primitive

## Purpose

A primitive is a reusable visual building block that knows nothing about the domain. It takes plain values as parameters, renders HTML, and is consumed by any number of pages and fragments. The shared page layout, the root `<html>` wrapper, modal shells, and icons are all primitives.

## Where it lives

`src/internal/core/templates/*.templ`. Multiple primitives can share a file when they're tightly related (e.g. a modal shell and its close-button helper); unrelated primitives get their own file.

## What a primitive cannot have

- **No domain types in its signature.** A primitive takes `string`, `int`, `bool`, slices of those, struct types defined in `core/templates`, or other primitives' parameter types — never a `TransactionDTO`, an `AccountDTO`, a `Classification`.
- **No imports of domain modules.** A primitive lives in `core/`; importing `transactions`, `budget`, `accounts`, etc. would create an inverted dependency.
- **No knowledge of an HTMX flow specific to one caller.** A primitive can emit HTMX attributes the caller supplies (e.g. a button primitive that accepts an `hx-post` URL), but it does not encode *"this is how the re-categorize flow works."*

## What a primitive should have

- A focused responsibility. Modal shell, not "the budget-editor modal." Button shape, not "the connect-bank button."
- A clear signature: a props struct when the parameter list exceeds a couple of values, positional params when it's one or two.
- An ID convention or naming helper exported alongside the templ when the primitive is HTMX-addressable from outside (e.g. the modal container's known DOM id).

## When something stops being a primitive

If a "primitive" needs a domain type to do its job, it isn't a primitive anymore — it's a fragment templ in the module that owns that domain type. Move it.

If two domain modules need slightly different versions of the same primitive, the primitive's parameters expand to cover both cases; the primitive does not branch on which module is calling it.

## Singleton: the shared layout

The page-level layout is a primitive like any other, but it has an additional responsibility: every page templ wraps in it. Changes to the layout primitive — new `<head>` tags, new scripts, new chrome — affect every page in the app. Treat edits to the layout primitive accordingly.
