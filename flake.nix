{
  description = "go-rns-pipe development shell";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };
  outputs =
    {
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.golangci-lint
          ];
          shellHook = ''
            echo "go-rns-pipe dev shell — go $(go version | awk '{print $3}')"
          '';
        };
      }
    );
}
