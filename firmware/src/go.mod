// This file exists only to mark firmware/src/ as a separate Go module
// boundary, so `go build/vet/test ./...` run from the AXOS repo root does
// not sweep in stray Go source (e.g. wireguard-tools/contrib/external-tests)
// that ships inside the vendored asuswrt-merlin.ng / am-toolchains
// submodules. Nothing here is meant to build — this module is never
// referenced by the root go.mod and has no dependents.
module github.com/reece01-dock/axos/firmware/src

go 1.24
