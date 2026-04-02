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
        orchid = pkgs.buildGoModule {
          pname = "orchid";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-KcwQhDiW2OjMw0OA0cYZGdLJhA+KrsBjH2WqeKHqU6U=";
          subPackages = [ "." ];

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
