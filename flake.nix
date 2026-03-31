{
  description = "Orchid VM manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
      orchid = pkgs.stdenv.mkDerivation {
        pname = "orchid";
        version = "0.1.0";
        src = ./.;
        nativeBuildInputs = [ pkgs.go ];

        buildPhase = ''
          runHook preBuild
          go build -o orchid .
          runHook postBuild
        '';

        installPhase = ''
          runHook preInstall
          mkdir -p $out/bin
          install -m755 orchid $out/bin/orchid
          runHook postInstall
        '';
      };
    in
    {
      packages.${system} = {
        default = orchid;
        orchid = orchid;
      };
      defaultPackage.${system} = orchid;

      apps.${system}.default = {
        type = "app";
        program = "${orchid}/bin/orchid";
      };

      nixosModules.default = { pkgs, ... }: {
        environment.systemPackages = [ self.packages.${pkgs.system}.default ];
      };

      devShells.${system}.default = pkgs.mkShell {
        packages = [
          pkgs.go
          pkgs.just
        ];
      };

      formatter.${system} = pkgs.nixfmt-rfc-style;
    };
}
