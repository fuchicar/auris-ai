{ pkgs ? import <nixpkgs> {} }:
pkgs.buildGoModule {
  pname   = "auris";
  version = "0.1.0";
  src     = ./.;

  subPackages = [ "cmd/auris" ];

  # Run `nix build` once with `null` to get the real hash from the error output,
  # then replace null with the printed sha256-... string.
  vendorHash = "sha256-KfaRH464QagazWenxS+KRztysXJF9Q+PdoNE5iCsX9Q=";

  meta = with pkgs.lib; {
    description = "Terminal-based financial AI advisor";
    mainProgram = "auris";
    platforms   = platforms.unix;
  };
}
