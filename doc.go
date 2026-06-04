// Package builtin provides ready-to-use Tool implementations for the
// agents subsystem: a hand-rolled arithmetic Calculator and an
// in-memory substring-match MockSearch.
//
// These tools have ZERO external dependencies (stdlib only) so the
// agents subsystem stays runnable in any environment without setup.
// MockSearch is explicitly NOT a real search engine — it is a
// deterministic substring matcher over an in-memory document slice,
// suitable for tests and learning demos only.
//
// # Portability
//
// builtin inherits the agents/pkg/llm portability contract — no
// imports from internal/*, no project-specific pkg/*, no business
// vocabulary in identifiers or strings.
package builtin
