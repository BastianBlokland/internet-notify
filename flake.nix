{
  description = "internet-notify";

  inputs = {
    nixpkgs.url = "nixpkgs/nixos-25.05";
  };

  outputs =
    { self, nixpkgs, ... }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
      tmpDir = "/tmp/internet-notify";
    in
    {
      packages.${system}.default = pkgs.buildGoModule {
        pname = "internet-notify";
        version = "0.1.0";
        src = ./.;
        vendorHash = "sha256-fqIa6n49xl3MgS4MShDsfVKyiQUt506XW0MYG8yrwwc=";
        proxyVendor = true;

        meta = {
          description = "A simple tool to give you Desktop Notifications about internet connectivity.";
          mainProgram = "internet-notify";
        };
      };

      defaultPackage.${system} = self.packages.${system};

      devShells.${system}.default = pkgs.mkShell {
        packages = [
          pkgs.nixfmt-rfc-style

          pkgs.go
          pkgs.gopls
          pkgs.go-tools
          pkgs.delve
        ];

        # Avoid polluting our home directory.
        GOPATH = "${tmpDir}/go";
        GOENV = "${tmpDir}/go/env";
        GOCACHE ="${tmpDir}/go/cache";
        GOMODCACHE = "${tmpDir}/go/pkg/mod";
        GOTELEMETRYDIR = "${tmpDir}/go/telemetry";
      };
    };
}
