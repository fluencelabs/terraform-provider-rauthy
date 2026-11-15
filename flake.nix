{
  description = "terraform-provider-rauthy — Terraform dev shell";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";

  outputs = { self, nixpkgs, ... }:
    let
      lib = nixpkgs.lib;
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];

      pkgsFor = system: import nixpkgs {
        inherit system;
        config.allowUnfreePredicate = pkg:
          builtins.elem (lib.getName pkg) [ "terraform" ];
      };

      forAllSystems = f: lib.genAttrs systems (s: f (pkgsFor s));
    in {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [ pkgs.terraform pkgs.go pkgs.terraform-plugin-docs pkgs.goreleaser pkgs.gnupg ];
          # go.mod asks for a newer Go than this nixpkgs ships; let the go
          # command fetch that toolchain instead of refusing to build.
          GOTOOLCHAIN = "auto";
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixpkgs-fmt);
    };
}
