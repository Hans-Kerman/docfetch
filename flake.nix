{
  description = "docfetch: clone only a git repo's documentation via sparse-checkout";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      forAllSystems = nixpkgs.lib.genAttrs nixpkgs.lib.systems.flakeExposed;
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.buildGo126Module {
            pname = "docfetch";
            version = "0.1.0";
            src = nixpkgs.lib.cleanSource ./.;
            vendorHash = null; # 零依赖，无 go.sum
            env.CGO_ENABLED = 0;
            nativeCheckInputs = [ pkgs.git ]; # TestRunGitPassesStderrThrough 需要 git
          };
        });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/docfetch";
        };
      });

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              git # docfetch 运行时 exec git
              go
              golangci-lint
              gopls
              gofumpt
            ];
          };
        });

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
