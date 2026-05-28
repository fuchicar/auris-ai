{ pkgs ? import <nixpkgs> {} }:
pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    gopls
    golangci-lint
    gotools
    git
  ];

  shellHook = ''
    echo "auris dev environment — $(go version)"
  '';
}
