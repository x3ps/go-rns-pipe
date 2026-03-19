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
            pkgs.gcc
            pkgs.go
            pkgs.gopls
            pkgs.golangci-lint
            pkgs.hugo
            (pkgs.python3.withPackages (ps: [
              ps.pytest
              ps.rns
            ]))
          ];
          shellHook = ''
            if [ ! -d "$PWD/.venv" ]; then
              python3 -m venv "$PWD/.venv"
            fi
            source "$PWD/.venv/bin/activate"
            # rns package not in nixpkgs; install manually: pip install rns
            echo "go-rns-pipe dev shell — go $(go version | awk '{print $3}')"
          '';
        };
      }
    );
}
