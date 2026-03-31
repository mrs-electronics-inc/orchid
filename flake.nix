{
  description = "Orchid VM manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        orchid = pkgs.stdenv.mkDerivation {
          pname = "orchid";
          version = "0.1.0";
          src = ./.;
          nativeBuildInputs = [ pkgs.go ];

          buildPhase = ''
            runHook preBuild
            export HOME="$TMPDIR"
            export CGO_ENABLED=0
            export GOCACHE="$TMPDIR/go-cache"
            mkdir -p "$GOCACHE"
            go build -trimpath -buildvcs=false -o orchid .
            runHook postBuild
          '';

          installPhase = ''
            runHook preInstall
            mkdir -p $out/bin
            install -m755 orchid $out/bin/orchid
            runHook postInstall
          '';

          meta = with pkgs.lib; {
            description = "Orchid VM manager";
            homepage = "https://github.com/mrs-electronics-inc/orchid";
            license = licenses.mit;
            platforms = platforms.all;
          };
        };
      in
      {
        packages = {
          default = orchid;
          orchid = orchid;
        };

        apps.default = {
          type = "app";
          program = "${orchid}/bin/orchid";
        };

        nixosModules.default = { pkgs, ... }: {
          environment.systemPackages = [ self.packages.${pkgs.system}.default ];
        };

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.just
          ];
        };

        formatter = pkgs.nixfmt-rfc-style;
      }
    );
}
