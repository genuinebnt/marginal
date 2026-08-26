//go:build !(js && wasm)

// Package main's real implementation (main.go, json.go) only compiles
// under GOOS=js GOARCH=wasm, since it imports syscall/js. This stub exists
// so `go build ./...`, `go vet ./...`, and `go test ./...` from the module
// root succeed under the host GOOS/GOARCH too, instead of reporting
// cmd/wasm as unbuildable — a standard pattern for a wasm-only command
// living inside an otherwise normal module.
package main

func main() {}
