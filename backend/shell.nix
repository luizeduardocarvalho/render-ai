# Dev shell for the Go backend. `go` is not installed globally on this machine
# (it's provided through Nix), so this pins it for the backend.
#
#   cd backend && nix-shell                      # drops you into a shell with `go`
#   cd backend && nix-shell --run "go run ./cmd/server"   # one-off
{ pkgs ? import <nixpkgs> { } }:

pkgs.mkShell {
  buildInputs = [ pkgs.go ];
}
