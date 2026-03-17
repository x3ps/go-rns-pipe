{
  description = "go-rns-pipe — pipeline primitives for RNS data processing";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      gomod2nix,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        gomod2nixPkgs = gomod2nix.legacyPackages.${system};
      in
      {
        packages.rns-tcp-iface = pkgs.buildGoModule {
          pname = "rns-tcp-iface";
          version = "0.0.0";
          src = ./.;
          modRoot = "examples/tcp";
          vendorHash = null;
          proxyVendor = true;
        };

        packages.default = self.packages.${system}.rns-tcp-iface;

        checks.default = pkgs.stdenvNoCC.mkDerivation {
          name = "go-rns-pipe-checks";
          src = ./.;
          nativeBuildInputs = [ pkgs.go ];
          doCheck = true;
          buildPhase = ''
            export HOME=$TMPDIR
            export GOPATH=$TMPDIR/go
            export GOCACHE=$TMPDIR/go-cache
            export CGO_ENABLED=0
            go test ./...
            go vet ./...
            cd examples/tcp && go vet ./...
          '';
          installPhase = ''
            touch $out
          '';
        };

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.golangci-lint
            pkgs.go-task
            gomod2nixPkgs.gomod2nix
            pkgs.git
          ];
        };
      }
    );
}
