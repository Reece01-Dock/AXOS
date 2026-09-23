// This file exists only to mark firmware/src-stock/ as a separate Go module
// boundary, so `go build/vet/test ./...` run from the AXOS repo root does
// not sweep in stray Go source (e.g. wireguard-tools/contrib/external-tests)
// that ships inside the local stock Merlin clone. Same purpose as
// firmware/src/go.mod. Nothing here is meant to build.
module github.com/reece01-dock/axos/firmware/src-stock

go 1.24
