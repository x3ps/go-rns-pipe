{
  description = "go-rns-pipe — pipeline primitives for RNS data processing";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    # gomod2nix is NOT used for building (we use pkgs.stdenv.mkDerivation).
    # It is included solely to provide the `gomod2nix` CLI tool in devShell
    # for updating gomod2nix.toml when dependencies change.
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
        packages.rns-tcp-iface = pkgs.stdenv.mkDerivation {
          pname = "rns-tcp-iface";
          version = "0.0.0";
          src = ./.;
          nativeBuildInputs = [ pkgs.go ];
          buildPhase = ''
            export HOME=$TMPDIR
            export GOPATH=$TMPDIR/go
            export GOCACHE=$TMPDIR/go-cache
            (cd examples/tcp && go build -o $TMPDIR/rns-tcp-iface .)
          '';
          installPhase = ''
            mkdir -p $out/bin
            cp $TMPDIR/rns-tcp-iface $out/bin/
          '';
        };

        packages.rns-udp-iface = pkgs.stdenv.mkDerivation {
          pname = "rns-udp-iface";
          version = "0.0.0";
          src = ./.;
          nativeBuildInputs = [ pkgs.go ];
          buildPhase = ''
            export HOME=$TMPDIR
            export GOPATH=$TMPDIR/go
            export GOCACHE=$TMPDIR/go-cache
            (cd examples/udp && go build -o $TMPDIR/rns-udp-iface .)
          '';
          installPhase = ''
            mkdir -p $out/bin
            cp $TMPDIR/rns-udp-iface $out/bin/
          '';
        };

        packages.default = self.packages.${system}.rns-udp-iface;

        checks.default = pkgs.stdenv.mkDerivation {
          name = "go-rns-pipe-checks";
          src = ./.;
          nativeBuildInputs = [
            pkgs.go
            pkgs.golangci-lint
          ];
          doCheck = true;
          buildPhase = ''
            export HOME=$TMPDIR
            export GOPATH=$TMPDIR/go
            export GOCACHE=$TMPDIR/go-cache
            export GOLANGCI_LINT_CACHE=$TMPDIR/golangci-lint-cache
            # Root module
            go test ./...
            go test -race ./...
            go vet ./...
            golangci-lint run

            # TCP example module
            cd examples/tcp
            go test ./...
            go test -race ./...
            go vet ./...
            golangci-lint run

            # UDP example module
            cd ../udp
            go test ./...
            go test -race ./...
            go vet ./...
            golangci-lint run
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
            pkgs.python3 # for `go test -tags=integration`; install rns: python3 -m pip install rns
          ];
        };
      }
    );
}
