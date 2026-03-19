// Package code provides code-aware sparse vector generation for source code.
//
// The CodeSparseProvider splits identifiers (camelCase, snake_case, acronyms)
// before hashing into sparse vectors, improving recall for code search.
package code
