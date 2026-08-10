module github.com/Cyberlane/mori

go 1.23.0

toolchain go1.26.5

require (
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/tree-sitter/go-tree-sitter v0.25.0
	github.com/tree-sitter/tree-sitter-bash v0.25.1
	github.com/tree-sitter/tree-sitter-c-sharp v0.23.5
	github.com/tree-sitter/tree-sitter-go v0.25.0
	github.com/tree-sitter/tree-sitter-java v0.23.5
	github.com/tree-sitter/tree-sitter-javascript v0.25.0
	github.com/tree-sitter/tree-sitter-python v0.25.0
	github.com/tree-sitter/tree-sitter-rust v0.24.2
	github.com/tree-sitter/tree-sitter-typescript v0.23.2
	github.com/tree-sitter/tree-sitter-zsh v0.63.5
	github.com/wippyai/tree-sitter-sql v0.0.4
)

require github.com/mattn/go-pointer v0.0.1 // indirect

// The tagged Zsh binding imports this historical path in its own Go test.
replace github.com/tree-sitter/tree-sitter-zsh => github.com/georgeharker/tree-sitter-zsh v0.63.5
