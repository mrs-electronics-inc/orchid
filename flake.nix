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
        version = builtins.replaceStrings ["\n"] [""] (builtins.readFile ./VERSION);
        commit = if self ? rev then builtins.substring 0 7 self.rev else "unknown";
        orchid = pkgs.buildGoModule {
          pname = "orchid";
          inherit version;
          src = ./.;
          vendorHash = "sha256-KcwQhDiW2OjMw0OA0cYZGdLJhA+KrsBjH2WqeKHqU6U=";
          subPackages = [ "./cmd/orchid" ];
          ldflags = [
            "-s -w"
            "-X github.com/mrs-electronics-inc/orchid/cmd/orchid.version=${version}"
            "-X github.com/mrs-electronics-inc/orchid/cmd/orchid.commit=${commit}"
          ];

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
